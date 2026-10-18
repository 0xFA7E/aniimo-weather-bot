package app

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xFA7E/aniimo-weather-bot/internal/vision"
)

func TestFormatConsoleForecast(t *testing.T) {
	got := formatConsoleForecast("Friend One", vision.Extraction{
		Region: "The Argent Strait", Timezone: "UTC-4",
		Rows: []vision.ForecastRow{
			{Day: "today", Start: "05:00 PM", End: "08:59 PM", Weather: "special_unknown"},
			{Day: "today", Start: "09:00 PM", End: "12:59 AM", Weather: "sunny"},
		},
	})
	for _, want := range []string{"SPECIAL WEATHER FORECAST IDENTIFIED", "Friend One", "The Argent Strait (UTC-4)", "Today 05:00 PM - 08:59 PM: special_unknown"} {
		if !contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if contains(got, "09:00 PM") {
		t.Errorf("sunny row included in special summary:\n%s", got)
	}
}

func TestFirstRunBaselinesExistingScreenshots(t *testing.T) {
	root := t.TempDir()
	screenshotDir := filepath.Join(root, "760", "remote", steamAppID, "screenshots")
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJPEG(t, filepath.Join(screenshotDir, "old.jpg"))
	dataDir := filepath.Join(t.TempDir(), "data")
	cfg := Config{ScreenshotDirs: []string{root}, DataDir: dataDir, PollSeconds: 1}
	if err := Run(context.Background(), cfg, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "observations.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("first run processed an existing screenshot: %v", err)
	}
}

func TestScanOnlyProcessesScreenshotInsideCursorWindow(t *testing.T) {
	root := t.TempDir()
	screenshotDir := filepath.Join(root, "760", "remote", steamAppID, "screenshots")
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(screenshotDir, "old.jpg")
	newPath := filepath.Join(screenshotDir, "new.jpg")
	writeTestJPEG(t, oldPath)
	writeTestJPEG(t, newPath)
	now := time.Now()
	if err := os.Chtimes(oldPath, now.Add(-time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, now, now); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	cfg := Config{ScreenshotDirs: []string{root}, DataDir: dataDir}
	reminders, err := loadReminders(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := scan(cfg, map[string]bool{}, reminders, now.Add(-time.Second), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dataDir, "observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" || contains(string(b), oldPath) || !contains(string(b), newPath) {
		t.Fatalf("unexpected observations: %s", b)
	}
}

func writeTestJPEG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: 30, B: 40, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func contains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
