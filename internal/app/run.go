package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xFA7E/aniimo-weather-bot/internal/vision"
)

const steamAppID = "4126040"

type observation struct {
	ObservedAt  time.Time             `json:"observed_at"`
	ModifiedAt  time.Time             `json:"screenshot_modified_at"`
	Path        string                `json:"path"`
	SHA256      string                `json:"sha256"`
	Width       int                   `json:"width"`
	Height      int                   `json:"height"`
	Status      string                `json:"status"`
	Reason      string                `json:"reason"`
	Classifier  vision.Classification `json:"classifier"`
	Extraction  *vision.Extraction    `json:"extraction,omitempty"`
	AlertStatus string                `json:"alert_status,omitempty"`
}

type scannerState struct {
	ProcessAfter time.Time `json:"process_after"`
}

type screenshotFile struct {
	Path       string
	ModifiedAt time.Time
}

func Run(ctx context.Context, cfg Config, once bool) error {
	logFile, logger, err := openDiagnosticLog(cfg.LogPath)
	if err != nil {
		return fmt.Errorf("open diagnostic log: %w", err)
	}
	if logFile != nil {
		defer logFile.Close()
		originalStatus := cfg.Status
		cfg.Status = func(message string) {
			logger.Print(message)
			if originalStatus != nil {
				originalStatus(message)
			}
		}
		logger.Printf("monitor starting; screenshot roots: %q", cfg.ScreenshotDirs)
	}
	status(cfg, "Starting screenshot monitor")
	watchDirs := aniimoScreenshotDirs(cfg.ScreenshotDirs)
	monitorStatus := "No Aniimo screenshot folder found — check Steam userdata folders in Advanced"
	if len(watchDirs) == 0 {
		status(cfg, monitorStatus)
	} else {
		monitorStatus = fmt.Sprintf("Watching %d Aniimo screenshot folder(s)", len(watchDirs))
		status(cfg, monitorStatus)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	seen, err := loadSeen(filepath.Join(cfg.DataDir, "observations.jsonl"))
	if err != nil {
		return err
	}
	reminders, err := loadReminders(cfg.DataDir)
	if err != nil {
		return err
	}
	if err := reminders.rebuildFromObservations(cfg, filepath.Join(cfg.DataDir, "observations.jsonl"), time.Now()); err != nil {
		return fmt.Errorf("rebuild reminders: %w", err)
	}
	if err := reminders.reconfigure(cfg, time.Now()); err != nil {
		return fmt.Errorf("update reminder settings: %w", err)
	}
	if err := reminders.dispatchDue(ctx, cfg, time.Now()); err != nil {
		return fmt.Errorf("deliver pending reminder: %w", err)
	}
	statePath := filepath.Join(cfg.DataDir, "state.json")
	state, exists, err := loadState(statePath)
	if err != nil {
		return err
	}
	if !exists {
		state.ProcessAfter = time.Now()
		if err := saveState(statePath, state); err != nil {
			return err
		}
		fmt.Printf("initialized screenshot baseline at %s; existing screenshots will not be processed\n", state.ProcessAfter.Format(time.RFC3339))
		status(cfg, monitorStatus)
		if once {
			return nil
		}
	}
	for {
		if err := reminders.dispatchDue(ctx, cfg, time.Now()); err != nil {
			return fmt.Errorf("deliver pending reminder: %w", err)
		}
		processedThrough := time.Now().Add(-time.Second)
		if processedThrough.After(state.ProcessAfter) {
			if err := scan(cfg, seen, reminders, state.ProcessAfter, processedThrough, logger); err != nil {
				return err
			}
			state.ProcessAfter = processedThrough
			if err := saveState(statePath, state); err != nil {
				return err
			}
		}
		if once {
			return nil
		}
		select {
		case <-ctx.Done():
			status(cfg, "Monitor stopped")
			return nil
		case <-time.After(time.Duration(cfg.PollSeconds) * time.Second):
		}
	}
}

func scan(cfg Config, seen map[string]bool, reminders *reminderStore, after, through time.Time, logger *log.Logger) error {
	files, err := screenshotFiles(cfg.ScreenshotDirs)
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].ModifiedAt.Equal(files[j].ModifiedAt) {
			return files[i].Path < files[j].Path
		}
		return files[i].ModifiedAt.Before(files[j].ModifiedAt)
	})
	for _, file := range files {
		if !file.ModifiedAt.After(after) || file.ModifiedAt.After(through) {
			continue
		}
		obs, img, err := inspect(file.Path)
		if err != nil {
			continue // Steam may still be writing the JPEG.
		}
		obs.ModifiedAt = file.ModifiedAt
		if seen[obs.SHA256] {
			continue
		}
		if obs.Classifier.Candidate {
			extractionCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			extraction, extractErr := vision.Extract(extractionCtx, img, cfg.TesseractPath)
			cancel()
			if extractErr != nil {
				obs.Status = "extraction_error"
				obs.Reason = obs.Classifier.Reason() + "; " + extractErr.Error()
			} else {
				obs.Extraction = &extraction
				obs.Reason = obs.Classifier.Reason() + "; " + extraction.Reason
				if !extraction.Valid {
					obs.Status = "ignored_unvalidated"
				} else if hasSpecial(extraction.Rows) {
					obs.Status = "weather_special"
					fmt.Print(formatConsoleForecast(cfg.SteamUserLabel, extraction))
					if err := reminders.addForecast(cfg, file.ModifiedAt, extraction); err != nil {
						return err
					}
					if cfg.DiscordWebhookURL == "" {
						obs.AlertStatus = "not_configured"
					} else if err := sendWeatherAlert(context.Background(), cfg.DiscordWebhookURL, cfg.SteamUserLabel, obs); err != nil {
						obs.AlertStatus = "failed"
						obs.Reason += "; Discord delivery failed: " + err.Error()
					} else {
						obs.AlertStatus = "sent"
					}
				} else {
					obs.Status = "weather_clear"
				}
			}
		} else {
			obs.Status = "ignored"
			obs.Reason = obs.Classifier.Reason()
		}
		if err := appendJSON(filepath.Join(cfg.DataDir, "observations.jsonl"), obs); err != nil {
			return err
		}
		seen[obs.SHA256] = true
		fmt.Printf("observed screenshot: %s (%s)\n", file.Path, obs.Reason)
		if logger != nil {
			logger.Printf("observed screenshot: %s; status=%s; %s", file.Path, obs.Status, obs.Reason)
		}
		if obs.Status == "weather_special" && obs.Extraction != nil {
			status(cfg, fmt.Sprintf("Special weather found in %s", obs.Extraction.Region))
		} else if obs.Status == "weather_clear" && obs.Extraction != nil {
			status(cfg, fmt.Sprintf("Forecast updated for %s", obs.Extraction.Region))
		} else if obs.Status == "extraction_error" {
			status(cfg, "Screenshot found, but OCR failed — check the Tesseract setting")
		} else if obs.Status == "ignored_unvalidated" {
			status(cfg, "Screenshot found, but the forecast text could not be read")
		} else {
			status(cfg, "Screenshot found, but it was not recognized as an Aniimo forecast")
		}
		if obs.AlertStatus == "failed" {
			delete(seen, obs.SHA256)
			return fmt.Errorf("Discord alert delivery failed; screenshot remains pending")
		}
	}
	return nil
}

func openDiagnosticLog(path string) (*os.File, *log.Logger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return f, log.New(f, "", log.Ldate|log.Ltime|log.Lmicroseconds), nil
}

func status(cfg Config, message string) {
	if cfg.Status != nil {
		cfg.Status(message)
	}
}

func screenshotFiles(roots []string) ([]screenshotFile, error) {
	var result []screenshotFile
	for _, root := range roots {
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			slash := filepath.ToSlash(path)
			if strings.Contains(slash, "/screenshots/thumbnails/") {
				return nil
			}
			if !strings.Contains(slash, "/760/remote/"+steamAppID+"/screenshots/") {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
				info, infoErr := d.Info()
				if infoErr == nil {
					result = append(result, screenshotFile{Path: path, ModifiedAt: info.ModTime()})
				}
			}
			return nil
		})
	}
	return result, nil
}

func aniimoScreenshotDirs(roots []string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, root := range roots {
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			slash := filepath.ToSlash(path)
			if strings.HasSuffix(strings.ToLower(slash), "/760/remote/"+steamAppID+"/screenshots") && !seen[path] {
				seen[path] = true
				result = append(result, path)
				return filepath.SkipDir
			}
			return nil
		})
	}
	sort.Strings(result)
	return result
}

func loadState(path string) (scannerState, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return scannerState{}, false, nil
	}
	if err != nil {
		return scannerState{}, false, err
	}
	var state scannerState
	if err := json.Unmarshal(b, &state); err != nil {
		return scannerState{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if state.ProcessAfter.IsZero() {
		return scannerState{}, false, nil
	}
	return state, true, nil
}

func saveState(path string, state scannerState) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func inspect(path string) (observation, image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return observation{}, nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return observation{}, nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return observation{}, nil, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return observation{}, nil, err
	}
	bounds := img.Bounds()
	classification := vision.Classify(img)
	return observation{
		ObservedAt: time.Now().UTC(), Path: path,
		SHA256: hex.EncodeToString(h.Sum(nil)), Width: bounds.Dx(), Height: bounds.Dy(),
		Classifier: classification,
	}, img, nil
}

func hasSpecial(rows []vision.ForecastRow) bool {
	for _, row := range rows {
		if row.Weather != "sunny" {
			return true
		}
	}
	return false
}

func formatConsoleForecast(userLabel string, extraction vision.Extraction) string {
	player := strings.TrimSpace(userLabel)
	if player == "" {
		player = "Unlabeled player"
	}
	region := strings.TrimSpace(extraction.Region)
	if region == "" {
		region = "Unknown region"
	}
	zone := strings.TrimSpace(extraction.Timezone)
	if zone != "" {
		zone = " (" + zone + ")"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nSPECIAL WEATHER FORECAST IDENTIFIED\n  Player: %s\n  Region: %s%s\n", player, region, zone)
	for _, row := range extraction.Rows {
		if row.Weather == "sunny" {
			continue
		}
		day := row.Day
		if day == "" {
			day = "forecast"
		}
		fmt.Fprintf(&b, "  - %s %s - %s: %s\n", strings.Title(day), row.Start, row.End, row.Weather)
	}
	b.WriteByte('\n')
	return b.String()
}

func loadSeen(path string) (map[string]bool, error) {
	seen := map[string]bool{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return seen, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var obs observation
		if err := dec.Decode(&obs); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if obs.AlertStatus != "failed" {
			seen[obs.SHA256] = true
		}
	}
	return seen, nil
}

func appendJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}
