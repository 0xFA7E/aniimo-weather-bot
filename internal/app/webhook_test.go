package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/0xFA7E/aniimo-weather-bot/internal/vision"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSendWeatherAlert(t *testing.T) {
	var got discordPayload
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Errorf("method=%s", req.Method)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type=%q", req.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	obs := observation{Extraction: &vision.Extraction{Region: "The Argent Strait", Timezone: "UTC-4", Rows: []vision.ForecastRow{
		{Day: "today", Start: "05:00 PM", End: "08:59 PM", Weather: "special_unknown"},
		{Day: "today", Start: "09:00 PM", End: "12:59 AM", Weather: "sunny"},
	}}}
	if err := sendWeatherAlertWithClient(context.Background(), client, "https://discord.com/api/webhooks/123/token", "Madf0x", obs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Content, "Player:** Madf0x") || !strings.Contains(got.Content, "The Argent Strait") || !strings.Contains(got.Content, "05:00 PM") {
		t.Fatalf("content=%q", got.Content)
	}
	if len(got.AllowedMentions.Parse) != 0 {
		t.Fatalf("allowed mentions=%v", got.AllowedMentions.Parse)
	}
}

func TestValidateDiscordWebhookURL(t *testing.T) {
	for _, bad := range []string{"", "http://discord.com/api/webhooks/1/x", "https://example.com/api/webhooks/1/x", "https://discord.com/channels/1/2"} {
		if err := validateDiscordWebhookURL(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
