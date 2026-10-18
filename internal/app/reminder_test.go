package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xFA7E/aniimo-weather-bot/internal/vision"
)

func TestReminderPersistsAndFiresAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	cfg := Config{DataDir: dataDir, SteamUserLabel: "Friend One", ReminderMinutesBefore: 5}
	zone := time.FixedZone("test", -7*60*60)
	captured := time.Date(2026, 10, 16, 20, 34, 33, 0, zone)
	extraction := vision.Extraction{Region: "Russet Highlands", CurrentTime: "04:34:33 AM", Rows: []vision.ForecastRow{
		{Day: "today", Start: "05:00 AM", End: "08:59 AM", Weather: "sunny"},
		{Day: "today", Start: "09:00 AM", End: "12:59 PM", Weather: "snowy"},
		{Day: "tomorrow", Start: "01:00 AM", End: "04:59 AM", Weather: "snowy"},
	}}
	store, err := loadReminders(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.addForecast(cfg, captured, extraction); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 2 {
		t.Fatalf("got %d reminders", len(store.items))
	}
	if want := time.Date(2026, 10, 17, 1, 0, 0, 0, zone); !store.items[0].EventAt.Equal(want) {
		t.Fatalf("event=%s want %s", store.items[0].EventAt, want)
	}

	reloaded, err := loadReminders(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.dispatchDue(context.Background(), cfg, reloaded.items[0].EventAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if reloaded.items[0].Status != "sent" {
		t.Fatalf("status=%q", reloaded.items[0].Status)
	}
	again, err := loadReminders(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if again.items[0].Status != "sent" {
		t.Fatalf("persisted status=%q", again.items[0].Status)
	}
}

func TestTomorrowFirstRowAnchorsToNextHour(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), ReminderMinutesBefore: 5}
	captured := time.Date(2026, 10, 16, 12, 50, 0, 0, time.Local)
	store, err := loadReminders(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	extraction := vision.Extraction{Region: "Mistwoods", CurrentTime: "08:50:00 PM", Rows: []vision.ForecastRow{{Day: "tomorrow", Start: "01:00 AM", End: "04:59 AM", Weather: "rainy"}}}
	if err := store.addForecast(cfg, captured, extraction); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 16, 17, 0, 0, 0, time.Local)
	if len(store.items) != 1 || !store.items[0].EventAt.Equal(want) {
		t.Fatalf("items=%+v want=%s", store.items, want)
	}
}

func TestMeasuredClockOffsetRejectsForecastRange(t *testing.T) {
	captured := time.Date(2026, 10, 17, 9, 33, 16, 0, time.Local)
	if got, ok := measuredClockOffset(captured, "05:33:15 PM"); !ok || got != 8*60 {
		t.Fatalf("got %d ok=%t", got, ok)
	}
	if _, ok := measuredClockOffset(captured, "05:00 PM"); ok {
		t.Fatal("accepted a range time without seconds")
	}
}

func TestGameClockSchedulesBeastRainAtOnePM(t *testing.T) {
	zone := time.FixedZone("PDT", -7*60*60)
	captured := time.Date(2026, 10, 17, 9, 33, 16, 0, zone)
	cfg := Config{DataDir: t.TempDir(), SteamUserLabel: "Test Player", ReminderMinutesBefore: 5}
	store, err := loadReminders(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	extraction := vision.Extraction{Region: "Beast Fang Ridge", CurrentTime: "05:33:15 PM", Rows: []vision.ForecastRow{
		{Day: "today", Start: "09:00 PM", End: "12:59 AM", Weather: "rainy"},
	}}
	if err := store.addForecast(cfg, captured, extraction); err != nil {
		t.Fatal(err)
	}
	wantEvent := time.Date(2026, 10, 17, 13, 0, 0, 0, zone)
	wantReminder := time.Date(2026, 10, 17, 12, 55, 0, 0, zone)
	if len(store.items) != 1 || !store.items[0].EventAt.Equal(wantEvent) || !store.items[0].RemindAt.Equal(wantReminder) {
		t.Fatalf("items=%+v want event=%s reminder=%s", store.items, wantEvent, wantReminder)
	}
}

func TestInvalidateLegacyPendingReminders(t *testing.T) {
	store := &reminderStore{path: filepath.Join(t.TempDir(), "reminders.json"), items: []reminder{
		{Version: 0, Status: "pending"}, {Version: 2, Status: "pending"}, {Version: 0, Status: "sent"},
	}}
	if err := store.invalidateLegacy(); err != nil {
		t.Fatal(err)
	}
	if store.items[0].Status != "invalidated" || store.items[1].Status != "invalidated" || store.items[2].Status != "sent" {
		t.Fatalf("items=%+v", store.items)
	}
}

func TestForecastRefreshReminderPersistsAndSupersedesOlderForecast(t *testing.T) {
	zone := time.FixedZone("PDT", -7*60*60)
	cfg := Config{DataDir: t.TempDir(), SteamUserLabel: "Player", ForecastRefreshReminder: true, ForecastRefreshMinutesBefore: 5}
	store, err := loadReminders(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	first := vision.Extraction{Region: "Mistwoods", CurrentTime: "05:30:00 PM", Rows: []vision.ForecastRow{
		{Day: "today", Start: "09:00 PM", End: "12:59 AM", Weather: "sunny"},
		{Day: "tomorrow", Start: "01:00 AM", End: "04:59 AM", Weather: "sunny"},
	}}
	captured := time.Date(2026, 10, 17, 9, 30, 0, 0, zone)
	if err := store.addForecast(cfg, captured, first); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 1 || store.items[0].Kind != "forecast_refresh" {
		t.Fatalf("items=%+v", store.items)
	}
	want := time.Date(2026, 10, 17, 21, 0, 0, 0, zone)
	if !store.items[0].EventAt.Equal(want) || !store.items[0].RemindAt.Equal(want.Add(-5*time.Minute)) {
		t.Fatalf("refresh=%+v want=%s", store.items[0], want)
	}

	newer := vision.Extraction{Region: "Mistwoods", CurrentTime: "09:30:00 PM", Rows: []vision.ForecastRow{{Day: "tomorrow", Start: "01:00 AM", End: "04:59 AM", Weather: "sunny"}, {Day: "tomorrow", Start: "05:00 AM", End: "08:59 AM", Weather: "sunny"}}}
	if err := store.addForecast(cfg, captured.Add(4*time.Hour), newer); err != nil {
		t.Fatal(err)
	}
	if store.items[0].Status != "superseded" || len(store.items) != 2 || store.items[1].Status != "pending" {
		t.Fatalf("items=%+v", store.items)
	}
}

func TestReconfigureDisablesAndUpdatesPendingReminders(t *testing.T) {
	now := time.Now()
	store := &reminderStore{path: filepath.Join(t.TempDir(), "reminders.json"), items: []reminder{
		{Version: 3, ID: "weather", Kind: "weather", EventAt: now.Add(time.Hour), EventEnds: now.Add(5 * time.Hour), RemindAt: now, Status: "pending"},
		{Version: 3, ID: "refresh", Kind: "forecast_refresh", EventAt: now.Add(2 * time.Hour), EventEnds: now.Add(6 * time.Hour), RemindAt: now, Status: "pending"},
	}}
	if err := store.reconfigure(Config{ReminderMinutesBefore: 0, ForecastRefreshReminder: true, ForecastRefreshMinutesBefore: 15}, now); err != nil {
		t.Fatal(err)
	}
	if store.items[0].Status != "disabled" {
		t.Fatalf("weather status=%s", store.items[0].Status)
	}
	if got, want := store.items[1].RemindAt, store.items[1].EventAt.Add(-15*time.Minute); !got.Equal(want) {
		t.Fatalf("refresh=%s want=%s", got, want)
	}
}

func TestCancelReminder(t *testing.T) {
	dir := t.TempDir()
	store := &reminderStore{path: filepath.Join(dir, "reminders.json"), clockPath: filepath.Join(dir, "clock.json"), items: []reminder{{ID: "one", Status: "pending"}}}
	if err := store.save(); err != nil {
		t.Fatal(err)
	}
	if err := CancelReminder(dir, "one"); err != nil {
		t.Fatal(err)
	}
	items, err := ListReminders(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("pending=%+v", items)
	}
}
