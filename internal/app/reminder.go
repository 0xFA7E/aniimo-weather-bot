package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xFA7E/aniimo-weather-bot/internal/vision"
)

type reminder struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Player    string    `json:"player"`
	Region    string    `json:"region"`
	Weather   string    `json:"weather"`
	EventAt   time.Time `json:"event_at"`
	EventEnds time.Time `json:"event_ends"`
	RemindAt  time.Time `json:"remind_at"`
	Status    string    `json:"status"`
	SentAt    time.Time `json:"sent_at,omitempty"`
	Kind      string    `json:"kind,omitempty"`
}

type ReminderInfo struct {
	ID, Player, Region, Weather, Kind, Status string
	EventAt, RemindAt                         time.Time
}

type reminderStore struct {
	path               string
	clockPath          string
	clockOffsetMinutes *int
	items              []reminder
}

type clockState struct {
	OffsetMinutes int `json:"game_clock_offset_minutes"`
}

func loadReminders(dataDir string) (*reminderStore, error) {
	store := &reminderStore{path: filepath.Join(dataDir, "reminders.json"), clockPath: filepath.Join(dataDir, "clock.json")}
	b, err := os.ReadFile(store.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &store.items); err != nil {
			return nil, fmt.Errorf("parse %s: %w", store.path, err)
		}
	}
	clockBytes, err := os.ReadFile(store.clockPath)
	if err == nil {
		var state clockState
		if err := json.Unmarshal(clockBytes, &state); err != nil {
			return nil, fmt.Errorf("parse %s: %w", store.clockPath, err)
		}
		store.clockOffsetMinutes = new(int)
		*store.clockOffsetMinutes = state.OffsetMinutes
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return store, nil
}

func (s *reminderStore) save() error {
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(s.path, b, 0o600)
}

func (s *reminderStore) addForecast(cfg Config, capturedAt time.Time, extraction vision.Extraction) error {
	if (cfg.ReminderMinutesBefore <= 0 && !cfg.ForecastRefreshReminder) || len(extraction.Rows) == 0 {
		return nil
	}
	if offset, ok := measuredClockOffset(capturedAt, extraction.CurrentTime); ok {
		if err := s.setClockOffset(offset); err != nil {
			return err
		}
	}
	if s.clockOffsetMinutes == nil {
		fmt.Printf("reminder not scheduled for %s: no trustworthy in-game clock calibration yet\n", extraction.Region)
		return nil
	}
	first, err := time.Parse("03:04 PM", extraction.Rows[0].Start)
	if err != nil {
		return nil
	}
	firstDayMinutes := 0
	if strings.EqualFold(extraction.Rows[0].Day, "tomorrow") {
		firstDayMinutes = 24 * 60
	}
	offset := time.Duration(*s.clockOffsetMinutes) * time.Minute
	gameNow := capturedAt.Add(offset)
	firstGameAt := time.Date(gameNow.Year(), gameNow.Month(), gameNow.Day(), first.Hour(), first.Minute(), 0, 0, gameNow.Location())
	if !firstGameAt.After(gameNow) {
		firstGameAt = firstGameAt.Add(24 * time.Hour)
	}
	anchor := firstGameAt.Add(-offset)
	existing := make(map[string]bool, len(s.items))
	for _, item := range s.items {
		if item.Version >= 3 && (item.Status == "pending" || item.Status == "sent") {
			existing[item.ID] = true
		}
	}
	changed := false
	var latestEnd time.Time
	for _, row := range extraction.Rows {
		start, err := time.Parse("03:04 PM", row.Start)
		if err == nil {
			rowDayMinutes := 0
			if strings.EqualFold(row.Day, "tomorrow") {
				rowDayMinutes = 24 * 60
			}
			deltaMinutes := rowDayMinutes + start.Hour()*60 + start.Minute() - (firstDayMinutes + first.Hour()*60 + first.Minute())
			if deltaMinutes < 0 {
				deltaMinutes += 24 * 60
			}
			end := anchor.Add(time.Duration(deltaMinutes) * time.Minute).Add(4 * time.Hour)
			if end.After(latestEnd) {
				latestEnd = end
			}
		}
		if cfg.ReminderMinutesBefore <= 0 {
			continue
		}
		if row.Weather == "sunny" {
			continue
		}
		start, err = time.Parse("03:04 PM", row.Start)
		if err != nil {
			continue
		}
		rowDayMinutes := 0
		if strings.EqualFold(row.Day, "tomorrow") {
			rowDayMinutes = 24 * 60
		}
		deltaMinutes := rowDayMinutes + start.Hour()*60 + start.Minute() - (firstDayMinutes + first.Hour()*60 + first.Minute())
		if deltaMinutes < 0 {
			deltaMinutes += 24 * 60
		}
		eventAt := anchor.Add(time.Duration(deltaMinutes) * time.Minute)
		region := extraction.Region
		if region == "" {
			region = "Unknown region"
		}
		id := fmt.Sprintf("%s|%s|%s|%s", cfg.SteamUserLabel, region, row.Weather, eventAt.UTC().Format(time.RFC3339))
		if existing[id] {
			continue
		}
		s.items = append(s.items, reminder{
			Version: 3, ID: id, Player: cfg.SteamUserLabel, Region: region, Weather: row.Weather,
			EventAt: eventAt, EventEnds: eventAt.Add(4 * time.Hour),
			RemindAt: eventAt.Add(-time.Duration(cfg.ReminderMinutesBefore) * time.Minute), Status: "pending", Kind: "weather",
		})
		existing[id], changed = true, true
	}
	if cfg.ForecastRefreshReminder && !latestEnd.IsZero() {
		region := extraction.Region
		if region == "" {
			region = "Unknown region"
		}
		for i := range s.items {
			if s.items[i].Kind == "forecast_refresh" && s.items[i].Player == cfg.SteamUserLabel && s.items[i].Region == region && s.items[i].Status == "pending" && s.items[i].EventAt.Before(latestEnd) {
				s.items[i].Status, changed = "superseded", true
			}
		}
		id := fmt.Sprintf("%s|%s|forecast_refresh|%s", cfg.SteamUserLabel, region, latestEnd.UTC().Format(time.RFC3339))
		if !existing[id] {
			s.items = append(s.items, reminder{Version: 3, ID: id, Player: cfg.SteamUserLabel, Region: region, Kind: "forecast_refresh", EventAt: latestEnd, EventEnds: latestEnd.Add(4 * time.Hour), RemindAt: latestEnd.Add(-time.Duration(cfg.ForecastRefreshMinutesBefore) * time.Minute), Status: "pending"})
			changed = true
		}
	}
	if !changed {
		return nil
	}
	sort.Slice(s.items, func(i, j int) bool { return s.items[i].EventAt.Before(s.items[j].EventAt) })
	return s.save()
}

func measuredClockOffset(capturedAt time.Time, current string) (int, bool) {
	gameClock, err := time.Parse("03:04:05 PM", strings.TrimSpace(current))
	if err != nil {
		return 0, false
	}
	minuteDifference := gameClock.Minute() - capturedAt.Minute()
	if minuteDifference < 0 {
		minuteDifference = -minuteDifference
	}
	if minuteDifference > 1 && minuteDifference < 59 {
		return 0, false
	}
	localSeconds := capturedAt.Hour()*3600 + capturedAt.Minute()*60 + capturedAt.Second()
	gameSeconds := gameClock.Hour()*3600 + gameClock.Minute()*60 + gameClock.Second()
	difference := gameSeconds - localSeconds
	for difference > 12*3600 {
		difference -= 24 * 3600
	}
	for difference <= -12*3600 {
		difference += 24 * 3600
	}
	return int(math.Round(float64(difference) / 60)), true
}

func (s *reminderStore) setClockOffset(minutes int) error {
	if s.clockOffsetMinutes != nil && *s.clockOffsetMinutes == minutes {
		return nil
	}
	s.clockOffsetMinutes = new(int)
	*s.clockOffsetMinutes = minutes
	b, err := json.MarshalIndent(clockState{OffsetMinutes: minutes}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.clockPath, append(b, '\n'), 0o600)
}

func (s *reminderStore) invalidateLegacy() error {
	changed := false
	for i := range s.items {
		if s.items[i].Version < 3 && s.items[i].Status == "pending" {
			s.items[i].Status, changed = "invalidated", true
		}
	}
	if changed {
		return s.save()
	}
	return nil
}

func (s *reminderStore) rebuildFromObservations(cfg Config, path string, now time.Time) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s.invalidateLegacy()
	}
	if err != nil {
		return err
	}
	defer f.Close()
	var observations []observation
	dec := json.NewDecoder(f)
	for {
		var obs observation
		if err := dec.Decode(&obs); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if obs.Extraction == nil || !obs.Extraction.Valid || obs.ModifiedAt.Before(now.Add(-30*time.Hour)) {
			continue
		}
		observations = append(observations, obs)
		if offset, ok := measuredClockOffset(obs.ModifiedAt, obs.Extraction.CurrentTime); ok {
			if err := s.setClockOffset(offset); err != nil {
				return err
			}
		}
	}
	if err := s.invalidateLegacy(); err != nil {
		return err
	}
	for _, obs := range observations {
		if !vision.IsKnownRegion(obs.Extraction.Region) {
			continue
		}
		clean := *obs.Extraction
		clean.Rows = nil
		for _, row := range obs.Extraction.Rows {
			if row.Weather != "special_unknown" {
				clean.Rows = append(clean.Rows, row)
			}
		}
		if hasSpecial(clean.Rows) {
			if err := s.addForecast(cfg, obs.ModifiedAt, clean); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *reminderStore) dispatchDue(ctx context.Context, cfg Config, now time.Time) error {
	changed := false
	for i := range s.items {
		item := &s.items[i]
		if item.Status != "pending" || now.Before(item.RemindAt) {
			continue
		}
		if !now.Before(item.EventEnds) {
			item.Status, changed = "expired", true
			continue
		}
		fmt.Print(formatReminder(*item))
		status(cfg, reminderStatus(*item))
		if cfg.DiscordWebhookURL != "" {
			if err := sendReminder(ctx, cfg.DiscordWebhookURL, *item); err != nil {
				if changed {
					_ = s.save()
				}
				return err
			}
		}
		item.Status, item.SentAt, changed = "sent", now, true
	}
	if changed {
		return s.save()
	}
	return nil
}

func (s *reminderStore) reconfigure(cfg Config, now time.Time) error {
	changed := false
	for i := range s.items {
		item := &s.items[i]
		if item.Status != "pending" && item.Status != "disabled" {
			continue
		}
		enabled, lead := cfg.ReminderMinutesBefore > 0, cfg.ReminderMinutesBefore
		if item.Kind == "forecast_refresh" {
			enabled, lead = cfg.ForecastRefreshReminder, cfg.ForecastRefreshMinutesBefore
		}
		if !enabled {
			if item.Status != "disabled" {
				item.Status, changed = "disabled", true
			}
			continue
		}
		if item.Status == "disabled" && now.Before(item.EventEnds) {
			item.Status, changed = "pending", true
		}
		newTime := item.EventAt.Add(-time.Duration(lead) * time.Minute)
		if !item.RemindAt.Equal(newTime) {
			item.RemindAt, changed = newTime, true
		}
	}
	if changed {
		return s.save()
	}
	return nil
}

func formatReminder(item reminder) string {
	player := strings.TrimSpace(item.Player)
	if player == "" {
		player = "Unlabeled player"
	}
	if item.Kind == "forecast_refresh" {
		return fmt.Sprintf("\nFORECAST REFRESH REMINDER\n  Player: %s\n  Region: %s\n  Forecast expires: %s\n  Take a new weather screenshot to continue alerts.\n\n", player, item.Region, item.EventAt.Format("Mon 03:04 PM"))
	}
	return fmt.Sprintf("\nSPECIAL WEATHER REMINDER\n  Player: %s\n  Region: %s\n  Starts: %s\n  Weather: %s\n\n",
		player, item.Region, item.EventAt.Format("Mon 03:04 PM"), item.Weather)
}

func reminderStatus(item reminder) string {
	if item.Kind == "forecast_refresh" {
		return "Forecast expired for " + item.Region
	}
	return fmt.Sprintf("%s starts in %s", item.Weather, item.Region)
}

func ListReminders(dataDir string) ([]ReminderInfo, error) {
	s, err := loadReminders(dataDir)
	if err != nil {
		return nil, err
	}
	result := make([]ReminderInfo, 0)
	for _, item := range s.items {
		if item.Status != "pending" {
			continue
		}
		result = append(result, ReminderInfo{ID: item.ID, Player: item.Player, Region: item.Region, Weather: item.Weather, Kind: item.Kind, Status: item.Status, EventAt: item.EventAt, RemindAt: item.RemindAt})
	}
	return result, nil
}

func CancelReminder(dataDir, id string) error {
	s, err := loadReminders(dataDir)
	if err != nil {
		return err
	}
	for i := range s.items {
		if s.items[i].ID == id && s.items[i].Status == "pending" {
			s.items[i].Status = "cancelled"
			return s.save()
		}
	}
	return fmt.Errorf("pending reminder not found")
}
