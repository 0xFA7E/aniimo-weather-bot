package gui

import (
	"context"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	weather "github.com/0xFA7E/aniimo-weather-bot/internal/app"
)

type controller struct {
	mu           sync.Mutex
	wg           sync.WaitGroup
	cancel       context.CancelFunc
	cfg          weather.Config
	configPath   string
	status       *widget.Label
	monitorState *widget.Label
	window       fyne.Window
	reminders    *fyne.Container
}

func Run(cfg weather.Config, configPath string) error {
	a := app.NewWithID("io.aniimo.weatherbot")
	a.Settings().SetTheme(newPastelDarkTheme())
	w := a.NewWindow("Aniimo Weather")
	w.Resize(fyne.NewSize(780, 680))
	state := widget.NewLabel("● Starting monitor…")
	state.Importance = widget.MediumImportance
	c := &controller{cfg: cfg, configPath: configPath, window: w, status: widget.NewLabel("Waiting for activity"), monitorState: state}

	content := c.build()
	w.SetContent(content)
	w.SetCloseIntercept(func() { w.Hide() })
	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayIcon(theme.InfoIcon())
		desk.SetSystemTrayMenu(fyne.NewMenu("Aniimo Weather",
			fyne.NewMenuItem("Open", func() { w.Show(); w.RequestFocus() }),
			fyne.NewMenuItem("Quit", func() { c.stop(); a.Quit() }),
		))
	}
	c.start()
	w.ShowAndRun()
	c.stop()
	return nil
}

func (c *controller) build() fyne.CanvasObject {
	title := widget.NewRichText(&widget.TextSegment{Text: "Aniimo Weather", Style: widget.RichTextStyleHeading})
	subtitle := widget.NewLabel("Your quiet companion for world-specific forecasts")
	subtitle.Importance = widget.LowImportance
	brandIcon := widget.NewIcon(theme.InfoIcon())
	brandIcon.SetResource(theme.InfoIcon())
	headerText := container.NewVBox(title, subtitle)
	header := coloredPanel(color.NRGBA{R: 0x27, G: 0x23, B: 0x42, A: 0xff}, container.NewBorder(nil, nil, brandIcon, nil, headerText))

	webhook := widget.NewPasswordEntry()
	webhook.SetText(c.cfg.DiscordWebhookURL)
	webhook.SetPlaceHolder("https://discord.com/api/webhooks/…")
	player := widget.NewEntry()
	player.SetText(c.cfg.SteamUserLabel)
	player.SetPlaceHolder("Steam or Discord name")
	weatherEnabled := widget.NewCheck("Remind me before special weather", nil)
	weatherEnabled.SetChecked(c.cfg.ReminderMinutesBefore > 0)
	weatherLead := widget.NewEntry()
	weatherLead.SetText(strconv.Itoa(max(c.cfg.ReminderMinutesBefore, 5)))
	weatherLead.Validator = nonNegativeInteger
	refreshEnabled := widget.NewCheck("Tell me when the saved forecast expires", nil)
	refreshEnabled.SetChecked(c.cfg.ForecastRefreshReminder)
	refreshLead := widget.NewEntry()
	refreshLead.SetText(strconv.Itoa(c.cfg.ForecastRefreshMinutesBefore))
	refreshLead.Validator = nonNegativeInteger

	screenshotDirs := widget.NewMultiLineEntry()
	screenshotDirs.SetText(strings.Join(c.cfg.ScreenshotDirs, "\n"))
	screenshotDirs.SetMinRowsVisible(3)
	tesseract := widget.NewEntry()
	tesseract.SetText(c.cfg.TesseractPath)
	tesseract.SetPlaceHolder("Bundled automatically, or choose a custom executable")
	diagnosticLog := widget.NewEntry()
	diagnosticLog.SetText(c.cfg.LogPath)
	diagnosticLog.SetPlaceHolder("Optional path, for example C:\\Users\\you\\Desktop\\aniimo-weather.log")

	form := widget.NewForm(
		widget.NewFormItem("Discord webhook", webhook),
		widget.NewFormItem("Player name", player),
		widget.NewFormItem("Weather lead (minutes)", weatherLead),
		widget.NewFormItem("Forecast lead (minutes)", refreshLead),
	)
	checks := container.NewVBox(weatherEnabled, refreshEnabled)
	advanced := widget.NewAccordion(widget.NewAccordionItem("Advanced", container.NewVBox(
		widget.NewLabel("Steam userdata folders (one per line)"), screenshotDirs,
		widget.NewLabel("Tesseract executable"), tesseract,
		widget.NewLabel("Diagnostic log file (optional)"), diagnosticLog,
	)))

	save := widget.NewButtonWithIcon("Save and restart monitor", theme.DocumentSaveIcon(), func() {
		weatherMinutes, err := strconv.Atoi(strings.TrimSpace(weatherLead.Text))
		if err != nil || weatherMinutes < 0 {
			c.setStatusUI("Weather lead time must be a non-negative number")
			return
		}
		refreshMinutes, err := strconv.Atoi(strings.TrimSpace(refreshLead.Text))
		if err != nil || refreshMinutes < 0 {
			c.setStatusUI("Forecast lead time must be a non-negative number")
			return
		}
		if !weatherEnabled.Checked {
			weatherMinutes = 0
		}
		dirs := splitLines(screenshotDirs.Text)
		next := c.cfg
		next.DiscordWebhookURL = strings.TrimSpace(webhook.Text)
		next.SteamUserLabel = strings.TrimSpace(player.Text)
		next.ReminderMinutesBefore = weatherMinutes
		next.ForecastRefreshReminder = refreshEnabled.Checked
		next.ForecastRefreshMinutesBefore = refreshMinutes
		next.ScreenshotDirs = dirs
		next.TesseractPath = strings.TrimSpace(tesseract.Text)
		next.LogPath = strings.TrimSpace(diagnosticLog.Text)
		if err := weather.SaveConfig(c.configPath, next); err != nil {
			c.setStatusUI("Could not save: " + err.Error())
			return
		}
		c.mu.Lock()
		c.cfg = next
		c.mu.Unlock()
		c.restart()
		c.setStatusUI("Settings saved; monitor restarted")
		c.refreshReminders()
	})
	save.Importance = widget.HighImportance
	test := widget.NewButtonWithIcon("Test webhook", theme.MailSendIcon(), func() {
		endpoint, identity := strings.TrimSpace(webhook.Text), strings.TrimSpace(player.Text)
		c.setStatusUI("Testing Discord webhook…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := weather.TestWebhook(ctx, endpoint, identity)
			if err != nil {
				c.setStatusAsync("Webhook test failed: " + err.Error())
			} else {
				c.setStatusAsync("Webhook test delivered")
			}
		}()
	})
	hide := widget.NewButtonWithIcon("Hide to tray", theme.MoveDownIcon(), func() { c.window.Hide() })
	hide.Importance = widget.LowImportance

	c.reminders = container.NewVBox()
	refreshList := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), c.refreshReminders)
	reminderIntro := widget.NewLabel("Upcoming alerts are stored on disk and survive restarts.")
	reminderIntro.Importance = widget.LowImportance
	reminderPage := container.NewVBox(container.NewBorder(nil, nil, nil, refreshList, reminderIntro), widget.NewSeparator(), c.reminders)

	activityCaption := widget.NewLabel("LATEST ACTIVITY")
	activityCaption.Importance = widget.LowImportance
	statusPanel := coloredPanel(color.NRGBA{R: 0x1b, G: 0x2b, B: 0x30, A: 0xff}, container.NewVBox(c.monitorState, activityCaption, c.status))
	settingsPage := container.NewVBox(
		sectionHeading("Discord identity", "Used to send alerts and distinguish players sharing a channel."),
		form,
		widget.NewSeparator(),
		sectionHeading("Notifications", "Choose which durable reminders the companion should schedule."),
		checks,
		widget.NewSeparator(),
		advanced,
		container.NewHBox(save, test, layout.NewSpacer(), hide),
	)
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), container.NewVScroll(settingsPage)),
		container.NewTabItemWithIcon("Reminders", theme.HistoryIcon(), container.NewVScroll(reminderPage)),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	root := container.NewBorder(container.NewVBox(header, statusPanel), nil, nil, nil, tabs)
	c.refreshReminders()
	return container.NewPadded(root)
}

func coloredPanel(fill color.Color, content fyne.CanvasObject) fyne.CanvasObject {
	background := canvas.NewRectangle(fill)
	background.CornerRadius = 14
	return container.NewStack(background, container.NewPadded(content))
}

func sectionHeading(title, subtitle string) fyne.CanvasObject {
	heading := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	detail := widget.NewLabel(subtitle)
	detail.Importance = widget.LowImportance
	detail.Wrapping = fyne.TextWrapWord
	return container.NewVBox(heading, detail)
}

func (c *controller) start() {
	c.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	cfg := c.cfg
	c.mu.Unlock()
	cfg.Status = c.setStatusAsync
	c.setMonitorStateUI("● Monitoring active", widget.SuccessImportance)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := weather.Run(ctx, cfg, false); err != nil && ctx.Err() == nil {
			c.setMonitorStateAsync("● Monitoring stopped by an error", widget.DangerImportance)
			c.setStatusAsync("Monitor error: " + err.Error())
		}
	}()
}

func (c *controller) stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.mu.Unlock()
	c.wg.Wait()
	c.setMonitorStateUI("● Monitoring stopped", widget.LowImportance)
}
func (c *controller) restart()                      { c.stop(); c.start() }
func (c *controller) setStatusUI(message string)    { c.status.SetText(message) }
func (c *controller) setStatusAsync(message string) { fyne.Do(func() { c.setStatusUI(message) }) }
func (c *controller) setMonitorStateUI(message string, importance widget.Importance) {
	c.monitorState.SetText(message)
	c.monitorState.Importance = importance
	c.monitorState.Refresh()
}
func (c *controller) setMonitorStateAsync(message string, importance widget.Importance) {
	fyne.Do(func() { c.setMonitorStateUI(message, importance) })
}

func (c *controller) refreshReminders() {
	c.mu.Lock()
	dataDir := c.cfg.DataDir
	c.mu.Unlock()
	items, err := weather.ListReminders(dataDir)
	if err != nil {
		c.setStatusUI("Could not load reminders: " + err.Error())
		return
	}
	c.reminders.RemoveAll()
	if len(items) == 0 {
		c.reminders.Add(widget.NewLabel("No upcoming reminders"))
		return
	}
	for _, item := range items {
		item := item
		label := item.Weather
		if item.Kind == "forecast_refresh" {
			label = "Refresh forecast"
		}
		when := fmt.Sprintf("%s · %s · alerts %s", item.Region, label, item.RemindAt.Format("Mon 3:04 PM"))
		cancel := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
			c.stop()
			if err := weather.CancelReminder(dataDir, item.ID); err != nil {
				c.start()
				c.setStatusUI("Could not cancel reminder: " + err.Error())
				return
			}
			c.start()
			c.setStatusUI("Reminder cancelled")
			c.refreshReminders()
		})
		c.reminders.Add(container.NewBorder(nil, nil, nil, cancel, widget.NewLabel(when)))
	}
	c.reminders.Refresh()
}

func splitLines(value string) []string {
	var result []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

func nonNegativeInteger(value string) error {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return fmt.Errorf("enter zero or a positive whole number")
	}
	return nil
}
