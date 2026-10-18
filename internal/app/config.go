package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ScreenshotDirs               []string     `json:"screenshot_dirs"`
	DataDir                      string       `json:"data_dir"`
	PollSeconds                  int          `json:"poll_seconds"`
	TesseractPath                string       `json:"tesseract_path,omitempty"`
	LogPath                      string       `json:"log_path,omitempty"`
	DiscordWebhookURL            string       `json:"discord_webhook_url,omitempty"`
	SteamUserLabel               string       `json:"steam_user_label,omitempty"`
	ReminderMinutesBefore        int          `json:"reminder_minutes_before"`
	ForecastRefreshReminder      bool         `json:"forecast_refresh_reminder"`
	ForecastRefreshMinutesBefore int          `json:"forecast_refresh_minutes_before"`
	Status                       func(string) `json:"-"`
}

func LoadConfig(explicit string) (Config, string, error) {
	path := explicit
	if path == "" {
		local := "config.json"
		if _, err := os.Stat(local); err == nil {
			path = local
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, local, err
		} else {
			base, err := os.UserConfigDir()
			if err != nil {
				return Config{}, "", err
			}
			path = filepath.Join(base, "aniimo-weather-bot", "config.json")
		}
	}
	cfg := defaultConfig()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, path, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, path, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.ScreenshotDirs = mergePaths(cfg.ScreenshotDirs, defaultScreenshotDirs())
	if cfg.PollSeconds < 1 {
		cfg.PollSeconds = 2
	}
	if cfg.ReminderMinutesBefore < 0 {
		cfg.ReminderMinutesBefore = 0
	}
	if cfg.ForecastRefreshMinutesBefore < 0 {
		cfg.ForecastRefreshMinutesBefore = 0
	}
	if cfg.DiscordWebhookURL != "" {
		if err := validateDiscordWebhookURL(cfg.DiscordWebhookURL); err != nil {
			return Config{}, path, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	return cfg, path, nil
}

func EnsureConfig(path string, cfg Config) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func defaultConfig() Config {
	base, _ := os.UserConfigDir()
	return Config{
		ScreenshotDirs: defaultScreenshotDirs(),
		DataDir:        filepath.Join(base, "aniimo-weather-bot"), PollSeconds: 2,
		ReminderMinutesBefore: 5, ForecastRefreshMinutesBefore: 5,
	}
}

func mergePaths(configured, discovered []string) []string {
	result := make([]string, 0, len(configured)+len(discovered))
	seen := make(map[string]bool)
	for _, group := range [][]string{configured, discovered} {
		for _, path := range group {
			path = filepath.Clean(path)
			if path == "." || seen[path] {
				continue
			}
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

func SaveConfig(path string, cfg Config) error {
	if cfg.PollSeconds < 1 {
		return fmt.Errorf("poll interval must be at least one second")
	}
	if cfg.ReminderMinutesBefore < 0 || cfg.ForecastRefreshMinutesBefore < 0 {
		return fmt.Errorf("reminder lead times cannot be negative")
	}
	if cfg.DiscordWebhookURL != "" {
		if err := validateDiscordWebhookURL(cfg.DiscordWebhookURL); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
