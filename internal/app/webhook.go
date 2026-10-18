package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type discordPayload struct {
	Content         string          `json:"content"`
	Username        string          `json:"username,omitempty"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
}

func sendWeatherAlert(ctx context.Context, endpoint, userLabel string, obs observation) error {
	return sendWeatherAlertWithClient(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint, userLabel, obs)
}

func sendWeatherAlertWithClient(ctx context.Context, client *http.Client, endpoint, userLabel string, obs observation) error {
	return sendDiscordContent(ctx, client, endpoint, formatWeatherAlert(userLabel, obs))
}

func sendReminder(ctx context.Context, endpoint string, item reminder) error {
	return sendDiscordContent(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint, formatDiscordReminder(item))
}

func sendDiscordContent(ctx context.Context, client *http.Client, endpoint, content string) error {
	if err := validateDiscordWebhookURL(endpoint); err != nil {
		return err
	}
	payload := discordPayload{
		Content: content, Username: "Aniimo Weather",
		AllowedMentions: allowedMentions{Parse: []string{}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Discord returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	return nil
}

func TestWebhook(ctx context.Context, endpoint, player string) error {
	identity := strings.TrimSpace(player)
	if identity == "" {
		identity = "Unlabeled player"
	}
	return sendDiscordContent(ctx, &http.Client{Timeout: 15 * time.Second}, endpoint,
		fmt.Sprintf("**Aniimo Weather connection test**\nAlerts are configured for **%s**.", identity))
}

func formatDiscordReminder(item reminder) string {
	player := strings.TrimSpace(item.Player)
	if player == "" {
		player = "Unlabeled player"
	}
	if item.Kind == "forecast_refresh" {
		return fmt.Sprintf("**Aniimo forecast refresh**\n**Player:** %s\n**Region:** %s\nYour saved forecast has ended. Take a new weather screenshot to continue alerts.", player, item.Region)
	}
	return fmt.Sprintf("**Aniimo weather reminder**\n**Player:** %s\n**Region:** %s\n**Starts:** %s\n**Weather:** %s",
		player, item.Region, item.EventAt.Format("Mon 03:04 PM"), item.Weather)
}

func validateDiscordWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "discord.com") {
		return fmt.Errorf("discord_webhook_url must be an https://discord.com URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "webhooks" || parts[2] == "" || parts[3] == "" {
		return fmt.Errorf("discord_webhook_url is not a Discord webhook URL")
	}
	return nil
}

func formatWeatherAlert(userLabel string, obs observation) string {
	region := "Unknown region"
	zone := ""
	if obs.Extraction != nil {
		if strings.TrimSpace(obs.Extraction.Region) != "" {
			region = strings.TrimSpace(obs.Extraction.Region)
		}
		if strings.TrimSpace(obs.Extraction.Timezone) != "" {
			zone = " (" + strings.TrimSpace(obs.Extraction.Timezone) + ")"
		}
	}
	var b strings.Builder
	identity := strings.TrimSpace(userLabel)
	if identity == "" {
		identity = "Unlabeled player"
	}
	fmt.Fprintf(&b, "**Aniimo special weather detected**\n**Player:** %s\n**Region:** %s%s\n", identity, region, zone)
	if obs.Extraction != nil {
		for _, row := range obs.Extraction.Rows {
			if row.Weather == "sunny" {
				continue
			}
			day := row.Day
			if day == "" {
				day = "forecast"
			}
			fmt.Fprintf(&b, "- %s, %s–%s: %s\n", strings.Title(day), row.Start, row.End, row.Weather)
		}
	}
	return strings.TrimSpace(b.String())
}
