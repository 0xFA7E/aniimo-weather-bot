# Aniimo Weather Bot

[![CI](https://github.com/0xFA7E/aniimo-weather-bot/actions/workflows/ci.yml/badge.svg)](https://github.com/0xFA7E/aniimo-weather-bot/actions/workflows/ci.yml)

An unofficial, open-source companion for Aniimo. It does not modify the game,
read game memory, automate input, or authenticate with game services.

Cross-platform desktop companion that watches Steam screenshots, recognizes
Aniimo's weather forecast screen, records observations, and sends selected
weather alerts to Discord.

The program never reads or modifies game memory, sends game input, or performs
game-network authentication.

## Current status

Screenshot discovery, deduplication, append-only observation history, and a
resolution-independent weather-card classifier are implemented. Tesseract OCR
must recognize the forecast heading and at least four time rows before a
screenshot is accepted. Recognized weather labels currently include `sunny`,
`rainy`, `snowy`, and `thunderstorm`; an unfamiliar future icon remains
`special_unknown` rather than being mislabeled.

Forecast OCR is restricted to the time column so special-Aniimo cards cannot
confuse row recognition. Only intervals actually visible in the screenshot are
recorded.

Region, clock, and timezone OCR are best-effort metadata because the animated
header background can obscure them. Forecast validation does not depend on
those fields.

## Download for Windows

Windows users do not need Go, Python, or a separate OCR installation. Open the
repository's [Releases](https://github.com/0xFA7E/aniimo-weather-bot/releases)
page, download the latest `windows-x64.zip`, extract the complete folder, and
double-click `AniimoWeather.exe`. Keep the included `tesseract` folder beside
the executable.

Windows SmartScreen may warn about the executable because community releases
are not code-signed. Each release includes a SHA-256 checksum for verification.

## Linux prerequisite

Install Tesseract with English language data. On Debian/Ubuntu:

```sh
sudo apt install tesseract-ocr tesseract-ocr-eng
```

Developer builds can use `tesseract_path` to select a custom OCR executable.

## Development

```sh
git clone https://github.com/0xFA7E/aniimo-weather-bot.git
cd aniimo-weather-bot
cp config.example.json config.json
# Edit config.json, then:
go run .

go test ./...
go run . --once
go run . --headless
```

Running without flags opens the desktop interface. Closing its window hides it
to the system tray so screenshot monitoring continues; use **Quit** in the tray
menu to stop it. The settings screen edits the webhook, player label, reminder
lead times, screenshot locations, and Tesseract path without hand-editing JSON.
It also lists pending alerts and can cancel any one of them. Settings use the
desktop's system light/dark palette.

Advanced settings can optionally enable a timestamped diagnostic log. It records
monitor startup, discovered screenshot roots, screenshot paths, classification
results, and OCR errors for bug reports; the Discord webhook is never written
to this log.

`--headless` preserves terminal-only operation, and `--once` performs one scan
without opening the GUI.

[`config.example.json`](config.example.json) is a safe editable template. Fields
not present in it retain the platform defaults, including Steam screenshot
discovery and the application data directory. A `config.json` in the current
directory is loaded automatically. `--config PATH` remains available as an
explicit override; when neither exists, the platform configuration directory
is used.

On first run, a platform-appropriate `config.json` is created under the user's
configuration directory. Steam AppID `4126040` screenshot directories beneath
configured Steam `userdata` roots are discovered automatically. On Windows the
Steam install location is also read from the registry, so Steam installations
on a custom drive do not need to be entered manually. The activity panel reports
whether an Aniimo screenshot folder was found and whether each new screenshot
was recognized as a forecast.

To enable Discord alerts, add the webhook URL to that local file:

```json
{
  "discord_webhook_url": "https://discord.com/api/webhooks/WEBHOOK_ID/WEBHOOK_TOKEN",
  "steam_user_label": "Your Discord or Steam name",
  "reminder_minutes_before": 5,
  "forecast_refresh_reminder": true,
  "forecast_refresh_minutes_before": 5
}
```

Keep the other generated settings in the file. The webhook URL is a credential:
do not commit the local configuration or share the URL. If it is exposed,
regenerate it in Discord and replace the configured value.

Each unseen screenshot produces one line in `observations.jsonl`. Validated
screenshots receive `weather_clear` or `weather_special`; unrelated screenshots
fail closed. When `discord_webhook_url` is configured, a validated
`weather_special` observation posts its region and non-sunny time windows.
It also includes the installation's configured `steam_user_label`, allowing
several players to share one webhook and channel without mixing up whose world
produced the forecast. If omitted, the alert uses `Unlabeled player`.
The terminal prints `SPECIAL WEATHER FORECAST IDENTIFIED` with the same player,
region, timezone, and special time windows before attempting webhook delivery.
Mentions are disabled in generated messages. A failed delivery remains pending
and stops the collector so restarting it retries that screenshot rather than
silently losing the alert.

Each accepted special-weather window is also stored in `reminders.json` with an
absolute event time. By default, the program posts a second Discord and terminal
reminder five minutes before the weather starts. Set `reminder_minutes_before`
to another non-negative value; `0` disables reminders. Pending reminders are
loaded before screenshot scanning whenever the program starts. If the computer
was off at the reminder time, the reminder fires immediately after startup as
long as the four-hour weather window has not ended; older events are marked
expired. Repeated screenshots of the same player, region, weather, and event
time do not create duplicate reminders.

Reminder times are anchored using the in-game clock shown in screenshots, not
the computer's current hour. A trustworthy clock read is stored in `clock.json`
as the measured game-to-system offset and reused across restarts and screenshots
whose clock text is obscured. If no calibration is available yet, the program
prints that the reminder was not scheduled instead of guessing a time.

The optional forecast-refresh reminder is separate from special-weather alerts.
When enabled, it posts near the end of the newest captured forecast and asks the
player to take a fresh screenshot. A newer screenshot for the same player and
region supersedes the older refresh prompt. Both kinds of pending reminder are
durable across restarts and appear in the GUI, where they can be cancelled
individually.

## Packaging and releases

The GUI uses Fyne and builds natively on Linux and Windows. A native development
build is:

```sh
go build -o aniimo-weather-bot .
```

Fyne's desktop driver uses CGO, so producing a Windows executable on Linux also
requires a MinGW cross-compiler (or `fyne-cross`). Building on a Windows build
machine avoids that extra cross-toolchain requirement.

Pushing a version tag such as `v0.1.0` runs the release workflow on a Windows
runner. It publishes a Windows x64 zip containing the GUI application, the
English Tesseract OCR runtime, instructions, and a SHA-256 checksum.

The first launch writes `state.json` with the current time and deliberately
ignores screenshots that already exist. Later scans process only files newer
than the persisted cursor, including screenshots taken while the collector was
stopped. A one-second settling delay prevents reading a JPEG while Steam is
still writing it. Files do not need to be deleted between runs.

## OCR fixture tests

Real screenshots are not committed because they can expose player names and
account identifiers. The deterministic unit tests run in every clone. Optional
end-to-end OCR tests use local files described in
[`testdata/README.md`](testdata/README.md) and skip cleanly when those fixtures
are absent.

## License and trademarks

The source code is available under the [MIT License](LICENSE). Aniimo and its
artwork are the property of their respective owners. This project is not
affiliated with or endorsed by the game's developers or publisher.
