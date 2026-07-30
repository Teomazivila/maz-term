package ui

import (
	"fmt"
	"log/slog"
	"strings"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// notificationPageSize is how many notifications are loaded at once.
const notificationPageSize = 200

// loadNotifications reloads the notification list, honouring any active filters.
func (a *App) loadNotifications() {
	if a.Storage == nil {
		return
	}

	notifications, err := a.Storage.GetFilteredNotifications(
		notificationPageSize, true, a.activeSources, a.activeSeverities)
	if err != nil {
		a.setStatus("failed to load notifications: %v", err)
		slog.Default().Error("failed to load notifications", "error", err)
		return
	}

	a.Notifications = notifications
	if a.SelectedNotification >= len(a.Notifications) {
		a.SelectedNotification = max(len(a.Notifications)-1, 0)
	}

	a.updateTabBadges()
}

// loadNotificationFilters populates the available filter values.
//
// The values come from the store, so the filter UI offers exactly the sources
// and severities that actually occur. It previously used a fixed list that
// included sources the dashboard can never produce.
func (a *App) loadNotificationFilters() {
	if a.Storage == nil {
		return
	}

	sources, err := a.Storage.GetNotificationSources()
	if err != nil {
		slog.Default().Error("failed to load notification sources", "error", err)
	} else {
		a.NotificationSources = sources
	}

	severities, err := a.Storage.GetNotificationSeverities()
	if err != nil {
		slog.Default().Error("failed to load notification severities", "error", err)
	} else {
		a.NotificationSeverities = severities
	}
}

// toggleNotificationFilter enters or leaves filter mode, seeding the draft from
// the filters currently applied.
func (a *App) toggleNotificationFilter() {
	a.NotificationFilterMode = !a.NotificationFilterMode
	if !a.NotificationFilterMode {
		a.setStatus("filter mode closed")
		return
	}

	a.loadNotificationFilters()

	a.filter = filterDraft{
		Sources:    make(map[string]bool, len(a.NotificationSources)),
		Severities: make(map[string]bool, len(a.NotificationSeverities)),
	}
	for _, source := range a.activeSources {
		a.filter.Sources[source] = true
	}
	for _, severity := range a.activeSeverities {
		a.filter.Severities[severity] = true
	}

	a.setStatus("filter: Tab switches list, Space toggles, Enter applies, Esc cancels")
}

// handleNotificationFilterEvent drives filter mode.
//
// The selections live on App. Keeping them in a function-local struct meant every
// keystroke discarded them, so Enter always applied an empty filter.
func (a *App) handleNotificationFilterEvent(e ui.Event) {
	switch e.ID {
	case "<Escape>", "f":
		a.NotificationFilterMode = false
		a.setStatus("filter cancelled")

	case "<Enter>":
		a.activeSources = selected(a.filter.Sources, a.NotificationSources)
		a.activeSeverities = selected(a.filter.Severities, a.NotificationSeverities)
		a.NotificationFilterMode = false

		a.loadNotifications()

		switch {
		case len(a.activeSources) == 0 && len(a.activeSeverities) == 0:
			a.setStatus("filters cleared")
		default:
			a.setStatus("filters applied: %d sources, %d severities",
				len(a.activeSources), len(a.activeSeverities))
		}

	case "<Tab>":
		a.filter.Section = (a.filter.Section + 1) % 2
		a.filter.Index = 0

	case "<Up>":
		if a.filter.Index > 0 {
			a.filter.Index--
		}

	case "<Down>":
		if a.filter.Index < len(a.currentFilterOptions())-1 {
			a.filter.Index++
		}

	case "<Space>":
		options := a.currentFilterOptions()
		if a.filter.Index < 0 || a.filter.Index >= len(options) {
			return
		}
		key := options[a.filter.Index]
		if a.filter.Section == 0 {
			a.filter.Sources[key] = !a.filter.Sources[key]
		} else {
			a.filter.Severities[key] = !a.filter.Severities[key]
		}

	case "<C-c>", "q":
		a.quit = true
	}
}

// currentFilterOptions returns the option list for the focused section.
func (a *App) currentFilterOptions() []string {
	if a.filter.Section == 0 {
		return a.NotificationSources
	}
	return a.NotificationSeverities
}

// updateNotificationsTabData refreshes the Notifications tab.
func (a *App) updateNotificationsTabData() {
	tab := a.getTabByName("Notifications")
	if tab == nil {
		return
	}

	a.refreshNotificationsIfDue()

	if len(tab.Widgets) == 0 {
		list := widgets.NewList()
		list.Title = "Notifications"
		list.WrapText = false
		list.BorderStyle.Fg = ui.ColorCyan
		list.TextStyle = normalStyle
		list.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorCyan)

		details := newPanel("Details")
		filters := newPanel("Filters")

		tab.Widgets = []ui.Drawable{list, details, filters}
		tab.Lists = []*widgets.List{list}
		tab.Panels = []*widgets.Paragraph{details, filters}
	}

	if len(tab.Lists) > 0 {
		list := tab.Lists[0]
		rows := make([]string, 0, len(a.Notifications))

		for _, n := range a.Notifications {
			unread := " "
			if !n.Read {
				unread = "*"
			}
			link := ""
			if n.ActionURL != "" {
				link = " [link]"
			}

			// termui lists parse this inline style syntax; the previous
			// "[red]TEXT[-]" form is tview syntax and rendered literally.
			rows = append(rows, fmt.Sprintf("%s [%s](%s) %s  %s%s",
				unread,
				severityLabel(string(n.Severity)),
				severityStyleTag(string(n.Severity)),
				n.Timestamp.Format("15:04:05"),
				TruncateString(n.Title, 60),
				link))
		}

		if len(rows) == 0 {
			rows = append(rows, "  no notifications")
		}

		list.Rows = rows
		if len(a.Notifications) > 0 {
			list.SelectedRow = a.SelectedNotification
		} else {
			list.SelectedRow = 0
		}
	}

	a.updateNotificationDetails()
	a.updateFilterPanel()
}

// refreshNotificationsIfDue reloads from storage on the throttle's cadence.
func (a *App) refreshNotificationsIfDue() {
	if a.Storage == nil || !a.notificationsRefresh.ready() {
		return
	}
	a.loadNotifications()
}

// updateNotificationDetails refreshes the detail panel for the selection.
func (a *App) updateNotificationDetails() {
	tab := a.getTabByName("Notifications")
	if tab == nil || len(tab.Panels) == 0 {
		return
	}

	panel := tab.Panels[0]

	if len(a.Notifications) == 0 || a.SelectedNotification >= len(a.Notifications) {
		panel.Text = "No notification selected."
		return
	}

	n := a.Notifications[a.SelectedNotification]

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", n.Title)
	fmt.Fprintf(&b, "severity  %s\n", n.Severity)
	fmt.Fprintf(&b, "source    %s\n", n.Source)
	fmt.Fprintf(&b, "time      %s\n", formatTime(n.Timestamp))
	fmt.Fprintf(&b, "read      %t\n", n.Read)
	if len(n.Tags) > 0 {
		fmt.Fprintf(&b, "tags      %s\n", strings.Join(n.Tags, ", "))
	}
	if n.Message != "" {
		fmt.Fprintf(&b, "\n%s\n", n.Message)
	}
	if n.ActionURL != "" {
		fmt.Fprintf(&b, "\nlink  %s  (press o to open)\n", n.ActionURL)
	}

	panel.Text = b.String()
}

// updateFilterPanel refreshes the filter panel, showing either the interactive
// picker or a summary of the filters in force.
func (a *App) updateFilterPanel() {
	tab := a.getTabByName("Notifications")
	if tab == nil || len(tab.Panels) < 2 {
		return
	}

	panel := tab.Panels[1]

	if !a.NotificationFilterMode {
		// A real summary of what is applied. The panel previously consulted two
		// variables that were never assigned, so it always claimed no filters
		// were active.
		if len(a.activeSources) == 0 && len(a.activeSeverities) == 0 {
			panel.Text = "No active filters. Press f to filter."
			return
		}

		var b strings.Builder
		b.WriteString("Active filters\n")
		if len(a.activeSources) > 0 {
			fmt.Fprintf(&b, "  sources    %s\n", strings.Join(a.activeSources, ", "))
		}
		if len(a.activeSeverities) > 0 {
			fmt.Fprintf(&b, "  severities %s\n", strings.Join(a.activeSeverities, ", "))
		}
		fmt.Fprintf(&b, "  showing    %d notifications\n", len(a.Notifications))
		b.WriteString("\nPress f to change.")
		panel.Text = b.String()
		return
	}

	var b strings.Builder
	b.WriteString("Tab switches list, Space toggles, Enter applies\n\n")

	b.WriteString(a.renderFilterSection("Sources", a.NotificationSources, a.filter.Sources, 0))
	b.WriteString("\n")
	b.WriteString(a.renderFilterSection("Severities", a.NotificationSeverities, a.filter.Severities, 1))

	panel.Text = b.String()
}

// renderFilterSection renders one checkbox list for the filter picker.
func (a *App) renderFilterSection(title string, options []string, chosen map[string]bool, section int) string {
	var b strings.Builder

	marker := "  "
	if a.filter.Section == section {
		marker = "> "
	}
	fmt.Fprintf(&b, "%s%s\n", marker, title)

	if len(options) == 0 {
		b.WriteString("    (none recorded)\n")
		return b.String()
	}

	for i, option := range options {
		box := "[ ]"
		if chosen[option] {
			box = "[x]"
		}
		cursor := "  "
		if a.filter.Section == section && a.filter.Index == i {
			cursor = "->"
		}
		fmt.Fprintf(&b, "  %s %s %s\n", cursor, box, option)
	}

	return b.String()
}

// updateTabBadges marks the Notifications tab when unread items exist.
func (a *App) updateTabBadges() {
	if a.Storage == nil {
		return
	}

	unread, err := a.Storage.GetUnreadNotificationCount()
	if err != nil {
		slog.Default().Error("failed to count unread notifications", "error", err)
		return
	}

	if tab := a.getTabByName("Notifications"); tab != nil {
		tab.HasUnread = unread > 0
	}
}

// severityLabel renders a fixed-width severity label.
func severityLabel(severity string) string {
	label := strings.ToUpper(severity)
	if label == "" {
		label = "INFO"
	}
	if len(label) > 8 {
		label = label[:8]
	}
	return fmt.Sprintf("%-8s", label)
}

// severityStyleTag maps a severity to a termui inline style.
func severityStyleTag(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "fg:red,mod:bold"
	case "high", "error":
		return "fg:red"
	case "medium", "warning":
		return "fg:yellow"
	case "low":
		return "fg:blue"
	default:
		return "fg:green"
	}
}
