package ui

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	ui "github.com/gizak/termui/v3"
)

// handleEvent dispatches one terminal event.
//
// Modal states are handled first and each returns, so a key consumed by a modal
// cannot also fall through to the global bindings.
func (a *App) handleEvent(e ui.Event) {
	if a.addingAnnotation {
		a.handleAnnotationInput(e)
		return
	}
	if a.NotificationFilterMode {
		a.handleNotificationFilterEvent(e)
		return
	}
	if a.NotificationDetailMode {
		if a.handleNotificationDetailEvent(e) {
			return
		}
	}
	if a.ZoomMode {
		if a.handleZoomModeEvent(e) {
			return
		}
	}

	// Tab-specific handling. A consumed key returns rather than continuing into
	// the global switch, which previously let one keypress trigger two actions.
	switch a.activeTabName() {
	case "Plugins":
		if a.handlePluginEvents(e) {
			return
		}
	case "Notifications":
		if a.handleNotificationEvents(e) {
			return
		}
	case "History":
		if a.handleHistoryEvents(e) {
			return
		}
	}

	a.handleGlobalEvent(e)
}

// handleGlobalEvent handles bindings available on every tab.
func (a *App) handleGlobalEvent(e ui.Event) {
	switch e.ID {
	case "q", "<C-c>":
		a.quit = true

	case "?":
		a.ShowHelp = !a.ShowHelp

	case "<Resize>":
		// A checked assertion: termui delivers the payload as an any, and an
		// unchecked assertion panics if it is ever another type.
		if payload, ok := e.Payload.(ui.Resize); ok {
			a.TermWidth, a.TermHeight = payload.Width, payload.Height
		}

	case "<Tab>", "<Right>", "l", "n":
		a.changeTab(1)

	case "<BackTab>", "<Left>", "h", "p":
		a.changeTab(-1)

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		index, err := strconv.Atoi(e.ID)
		if err != nil {
			return
		}
		a.selectTab(index - 1)

	case "r":
		a.updateData()
		a.setStatus("refreshed")

	case "e":
		a.exportData()
	}
}

// handleHistoryEvents handles bindings specific to the History tab. It reports
// whether the event was consumed.
func (a *App) handleHistoryEvents(e ui.Event) bool {
	switch e.ID {
	case "[":
		a.setHistoryRange(a.clampedRangeIndex() - 1)
		return true

	case "]":
		a.setHistoryRange(a.clampedRangeIndex() + 1)
		return true

	case "a":
		a.ShowAnnotations = !a.ShowAnnotations
		a.setStatus("annotations %s", onOff(a.ShowAnnotations))
		return true

	case "A":
		a.beginAnnotation()
		return true

	case "z":
		a.enterZoomMode()
		return true
	}
	return false
}

// selectTab activates the tab at index when it exists.
func (a *App) selectTab(index int) {
	if index < 0 || index >= len(a.Tabs) {
		return
	}
	a.ActiveTabIndex = index
	a.onTabChanged()
}

// changeTab moves the selection by delta, wrapping at both ends.
func (a *App) changeTab(delta int) {
	if len(a.Tabs) == 0 {
		return
	}

	index := (a.ActiveTabIndex + delta) % len(a.Tabs)
	if index < 0 {
		index += len(a.Tabs)
	}
	a.ActiveTabIndex = index
	a.onTabChanged()
}

// onTabChanged runs the side effects of switching tabs.
func (a *App) onTabChanged() {
	tab := a.activeTab()
	if tab == nil {
		return
	}

	a.setStatus("%s", tab.Name)

	if tab.Name == "Notifications" {
		tab.HasUnread = false
		a.loadNotifications()
	}
}

// beginAnnotation opens the annotation form with an empty draft.
func (a *App) beginAnnotation() {
	a.addingAnnotation = true
	a.annotation = annotationDraft{}

	if a.AnnotationForm == nil {
		a.AnnotationForm = newPanel("Add annotation")
		a.AnnotationForm.BorderStyle.Fg = ui.ColorYellow
	}

	a.renderAnnotationForm()
	a.setStatus("annotation: enter a title")
}

// handleAnnotationInput accumulates text for the annotation form.
//
// The draft lives on App. Holding it in a function-local struct meant every
// keystroke reset it, so the title never exceeded one character and the form
// could not be submitted.
func (a *App) handleAnnotationInput(e ui.Event) {
	switch e.ID {
	case "<Escape>", "<C-c>":
		a.addingAnnotation = false
		a.annotation = annotationDraft{}
		a.setStatus("annotation cancelled")
		return

	case "<Enter>":
		if a.annotation.Field == 0 {
			if a.annotation.Title == "" {
				a.setStatus("annotation: a title is required")
				return
			}
			a.annotation.Field = 1
			a.setStatus("annotation: enter a description, then Enter to save")
		} else {
			a.submitAnnotation(a.annotation.Title, a.annotation.Description)
			a.addingAnnotation = false
			a.annotation = annotationDraft{}
			return
		}

	case "<Backspace>", "<C-8>":
		a.editDraft(func(s string) string {
			if s == "" {
				return s
			}
			runes := []rune(s)
			return string(runes[:len(runes)-1])
		})

	case "<Space>":
		a.editDraft(func(s string) string { return s + " " })

	default:
		// termui reports printable keys as single-character event IDs.
		if len([]rune(e.ID)) == 1 {
			a.editDraft(func(s string) string { return s + e.ID })
		}
	}

	a.renderAnnotationForm()
}

// editDraft applies edit to whichever annotation field has focus.
func (a *App) editDraft(edit func(string) string) {
	if a.annotation.Field == 0 {
		a.annotation.Title = edit(a.annotation.Title)
		return
	}
	a.annotation.Description = edit(a.annotation.Description)
}

// renderAnnotationForm refreshes the form's text from the current draft.
func (a *App) renderAnnotationForm() {
	if a.AnnotationForm == nil {
		return
	}

	cursor := func(field int) string {
		if a.annotation.Field == field {
			return "_"
		}
		return ""
	}

	a.AnnotationForm.Text = fmt.Sprintf(
		"Title\n  %s%s\n\nDescription\n  %s%s\n\nEnter to continue, Esc to cancel",
		a.annotation.Title, cursor(0),
		a.annotation.Description, cursor(1),
	)
}

// submitAnnotation stores the drafted annotation.
func (a *App) submitAnnotation(title, description string) {
	if a.Storage == nil {
		a.setStatus("cannot save annotation: storage is disabled")
		return
	}

	annotation := models.EventAnnotation{
		ID:          fmt.Sprintf("ann-%d", time.Now().UnixNano()),
		Title:       title,
		Description: description,
		Timestamp:   time.Now(),
		// EventTypeOther is a defined constant; the previous value "manual" was
		// not one of the EventType values at all.
		Type:     models.EventTypeOther,
		Severity: models.SeverityInfo,
		Source:   "operator",
		Tags:     []string{"user-created"},
	}

	if err := a.Storage.AddEventAnnotation(annotation); err != nil {
		a.setStatus("failed to save annotation: %v", err)
		slog.Default().Error("failed to save annotation", "error", err)
		return
	}

	a.refreshAnnotations()
	a.setStatus("annotation saved")
}

// enterZoomMode starts interactive zoom selection on the History tab.
func (a *App) enterZoomMode() {
	a.ZoomMode = true
	a.ZoomStartPercent = 0.25
	a.ZoomEndPercent = 0.75
	a.updateZoomIndicator()
}

// handleZoomModeEvent handles zoom selection keys, reporting whether the event
// was consumed.
func (a *App) handleZoomModeEvent(e ui.Event) bool {
	const step = 0.05

	switch e.ID {
	case "z", "<Escape>":
		a.ZoomMode = false
		a.setStatus("zoom cancelled")

	case "<Left>":
		if a.ZoomStartPercent-step >= 0 {
			a.ZoomStartPercent -= step
			a.ZoomEndPercent -= step
			a.updateZoomIndicator()
		}

	case "<Right>":
		if a.ZoomEndPercent+step <= 1 {
			a.ZoomStartPercent += step
			a.ZoomEndPercent += step
			a.updateZoomIndicator()
		}

	case "<Up>":
		if a.ZoomEndPercent-a.ZoomStartPercent > 2*step {
			a.ZoomStartPercent += step
			a.ZoomEndPercent -= step
			a.updateZoomIndicator()
		}

	case "<Down>":
		a.ZoomStartPercent = maxFloat(0, a.ZoomStartPercent-step)
		a.ZoomEndPercent = minFloat(1, a.ZoomEndPercent+step)
		a.updateZoomIndicator()

	case "<Enter>":
		a.applyZoom()

	default:
		return false
	}

	return true
}

// updateZoomIndicator recomputes the selected window from the percentages.
func (a *App) updateZoomIndicator() {
	end := time.Now()
	start := end.Add(-a.HistoryRange)
	span := end.Sub(start)

	a.ZoomStartTime = start.Add(time.Duration(float64(span) * a.ZoomStartPercent))
	a.ZoomEndTime = start.Add(time.Duration(float64(span) * a.ZoomEndPercent))

	a.setStatus("zoom %s to %s - Enter to apply, Esc to cancel",
		a.ZoomStartTime.Format("15:04"), a.ZoomEndTime.Format("15:04"))
}

// applyZoom narrows the history window to the selected span.
func (a *App) applyZoom() {
	span := a.ZoomEndTime.Sub(a.ZoomStartTime)
	if span <= 0 {
		a.setStatus("zoom window is empty")
		return
	}

	a.HistoryRange = span
	// A negative index marks a custom range that matches no preset.
	a.HistoryRangeIdx = -1
	a.ZoomMode = false

	a.setStatus("zoomed to %s", span.Round(time.Minute))
	a.updateHistoryTabData()
}

// exportData writes the recorded metrics to CSV.
//
// This previously reported "Export functionality not yet implemented" while a
// complete exporter sat unused in the storage adapter.
func (a *App) exportData() {
	if a.Storage == nil {
		a.setStatus("cannot export: storage is disabled")
		return
	}

	a.setStatus("exporting...")

	files, err := a.Storage.ExportAll("", a.HistoryRange)
	if err != nil {
		a.setStatus("export failed: %v", err)
		slog.Default().Error("export failed", "error", err)
		return
	}

	if len(files) == 0 {
		a.setStatus("nothing to export yet")
		return
	}

	a.setStatus("exported %d files to %s", len(files), filepath.Dir(files[0]))
	slog.Default().Info("exported metrics", "files", len(files))
}

// openURL opens a validated http(s) URL in the operator's browser.
//
// Only http and https are allowed. Handing an arbitrary string to the platform
// opener let a notification, which any plugin can create, point at a file:// URL
// or smuggle shell metacharacters through cmd.exe.
func (a *App) openURL(raw string) {
	parsed, err := url.Parse(raw)
	if err != nil {
		a.setStatus("cannot open link: %v", err)
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		a.setStatus("refusing to open %s link", parsed.Scheme)
		slog.Default().Warn("blocked non-http URL from notification", "scheme", parsed.Scheme)
		return
	}
	if parsed.Host == "" {
		a.setStatus("refusing to open link without a host")
		return
	}

	safe := parsed.String()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", safe)
	case "windows":
		// rundll32 receives the URL directly, so it is never re-parsed by
		// cmd.exe the way "cmd /c start <url>" would be.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", safe)
	default:
		cmd = exec.Command("xdg-open", safe)
	}

	if err := cmd.Start(); err != nil {
		a.setStatus("failed to open link: %v", err)
		slog.Default().Error("failed to open URL", "error", err)
		return
	}

	// The browser is not waited on, but the process is reaped so it does not
	// linger as a zombie for the dashboard's lifetime.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Default().Debug("browser process exited with an error", "error", err)
		}
	}()

	a.setStatus("opened %s", safe)
}

// handleNotificationEvents handles bindings specific to the Notifications tab,
// reporting whether the event was consumed.
func (a *App) handleNotificationEvents(e ui.Event) bool {
	switch e.ID {
	case "f":
		a.toggleNotificationFilter()

	case "d":
		a.NotificationDetailMode = !a.NotificationDetailMode
		a.updateNotificationDetails()

	case "<Up>":
		a.moveNotificationSelection(-1)

	case "<Down>":
		a.moveNotificationSelection(1)

	case "m":
		a.withSelectedNotification(func(n models.Notification) {
			if n.Read {
				a.setStatus("already read")
				return
			}
			a.applyNotificationAction("marked as read", a.Storage.MarkAsRead(n.ID))
		})

	case "D":
		a.withSelectedNotification(func(n models.Notification) {
			a.applyNotificationAction("dismissed", a.Storage.DismissNotification(n.ID))
		})

	case "C":
		if a.Storage == nil {
			a.setStatus("storage is disabled")
			return true
		}
		a.applyNotificationAction("all notifications cleared", a.Storage.ClearAllNotifications())

	case "o":
		a.withSelectedNotification(func(n models.Notification) {
			if n.ActionURL == "" {
				a.setStatus("no link on this notification")
				return
			}
			a.openURL(n.ActionURL)
		})

	default:
		return false
	}

	return true
}

// handleNotificationDetailEvent handles the detail overlay, reporting whether the
// event was consumed.
func (a *App) handleNotificationDetailEvent(e ui.Event) bool {
	switch e.ID {
	case "<Escape>":
		a.NotificationDetailMode = false
		a.setStatus("detail view closed")
		return true
	}
	return false
}

// withSelectedNotification runs fn against the highlighted notification.
func (a *App) withSelectedNotification(fn func(models.Notification)) {
	if a.Storage == nil {
		a.setStatus("storage is disabled")
		return
	}
	if len(a.Notifications) == 0 {
		a.setStatus("no notifications")
		return
	}
	if a.SelectedNotification < 0 || a.SelectedNotification >= len(a.Notifications) {
		a.SelectedNotification = 0
	}

	fn(a.Notifications[a.SelectedNotification])
}

// applyNotificationAction reports the outcome of a notification mutation and
// reloads the list on success.
func (a *App) applyNotificationAction(success string, err error) {
	switch {
	case err == nil:
		a.setStatus("%s", success)
		a.loadNotifications()
	case errors.Is(err, models.ErrNotFound):
		a.setStatus("notification no longer exists")
		a.loadNotifications()
	default:
		a.setStatus("action failed: %v", err)
		slog.Default().Error("notification action failed", "error", err)
	}
}

// moveNotificationSelection moves the highlight by delta, clamped to the list.
func (a *App) moveNotificationSelection(delta int) {
	if len(a.Notifications) == 0 {
		return
	}

	next := a.SelectedNotification + delta
	if next < 0 || next >= len(a.Notifications) {
		return
	}

	a.SelectedNotification = next
	a.setStatus("notification %d of %d", next+1, len(a.Notifications))

	if a.NotificationDetailMode {
		a.updateNotificationDetails()
	}
}

// formatTime renders a timestamp for detail views.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
