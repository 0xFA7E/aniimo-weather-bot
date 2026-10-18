package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigPrefersCurrentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"steam_user_label":"Local Player"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg, path, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if path != "config.json" {
		t.Fatalf("path=%q", path)
	}
	if cfg.SteamUserLabel != "Local Player" {
		t.Fatalf("label=%q", cfg.SteamUserLabel)
	}
	if len(cfg.ScreenshotDirs) == 0 {
		t.Fatal("platform defaults were not retained")
	}
}

func TestLoadConfigExplicitPathOverridesCurrentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	dir := t.TempDir()
	local := filepath.Join(dir, "config.json")
	explicit := filepath.Join(dir, "chosen.json")
	if err := os.WriteFile(local, []byte(`{"steam_user_label":"Local"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(explicit, []byte(`{"steam_user_label":"Explicit"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg, path, err := LoadConfig(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if path != explicit || cfg.SteamUserLabel != "Explicit" {
		t.Fatalf("path=%q label=%q", path, cfg.SteamUserLabel)
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := defaultConfig()
	want.SteamUserLabel = "GUI Player"
	want.ForecastRefreshReminder = true
	want.ForecastRefreshMinutesBefore = 10
	want.LogPath = filepath.Join(t.TempDir(), "weather.log")
	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SteamUserLabel != want.SteamUserLabel || !got.ForecastRefreshReminder || got.ForecastRefreshMinutesBefore != 10 || got.LogPath != want.LogPath {
		t.Fatalf("got=%+v", got)
	}
}
