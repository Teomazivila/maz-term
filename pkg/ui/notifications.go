package ui

import (
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// loadNotifications loads notifications from storage
func (a *App) loadNotifications() {
	if a.Storage == nil {
		return
	}

	// Get notifications from storage
	notifications, err := a.Storage.GetNotifications(100, false)
	if err != nil {
		a.StatusBar.Text = fmt.Sprintf("Error loading notifications: %s", err.Error())
		return
	}

	a.Notifications = notifications

	// Also update tab badges
	a.updateTabBadges()
}

// loadFilteredNotifications loads notifications with filters applied
func (a *App) loadFilteredNotifications(sources, severities []string) {
	if a.Storage == nil {
		return
	}

	// Get filtered notifications from storage
	notifications, err := a.Storage.GetFilteredNotifications(100, false, sources, severities)
	if err != nil {
		a.StatusBar.Text = fmt.Sprintf("Error loading notifications: %s", err.Error())
		return
	}

	a.Notifications = notifications

	// Update the UI to reflect applied filters
	filtersDesc := ""
	if len(sources) > 0 {
		filtersDesc += fmt.Sprintf("Sources: %v ", sources)
	}
	if len(severities) > 0 {
		filtersDesc += fmt.Sprintf("Severities: %v", severities)
	}

	if filtersDesc != "" {
		a.StatusBar.Text = fmt.Sprintf("Filters applied: %s", filtersDesc)
	} else {
		a.StatusBar.Text = "All filters cleared"
	}
}

// loadNotificationFilters loads available notification filters
func (a *App) loadNotificationFilters() {
	// This would typically come from the storage provider
	// For now, we'll just use a static list
	a.NotificationSources = []string{
		"system", "http", "git", "aws", "kubernetes", "cicd", "plugin",
	}

	a.NotificationSeverities = []string{
		"critical", "high", "medium", "low", "info",
	}
}

// updateNotificationsTabData updates the Notifications tab UI
func (a *App) updateNotificationsTabData() {
	// Get the notifications tab
	tab := a.getTabByName("Notifications")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create a list for notifications
		notificationsList := widgets.NewList()
		notificationsList.Title = "Notifications"
		notificationsList.WrapText = true
		notificationsList.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorCyan)

		// Create a paragraph for notification details
		notificationDetails := widgets.NewParagraph()
		notificationDetails.Title = "Details"
		notificationDetails.WrapText = true

		// Create a paragraph for filter display
		filterDisplay := widgets.NewParagraph()
		filterDisplay.Title = "Filters"
		filterDisplay.Text = "Press 'f' to filter notifications"

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{notificationsList, notificationDetails, filterDisplay}
		tab.Lists = []*widgets.List{notificationsList}
		tab.Panels = []*widgets.Paragraph{notificationDetails, filterDisplay}
	}

	// Update the notifications list
	list := tab.Lists[0]
	rows := []string{}

	if len(a.Notifications) == 0 {
		rows = append(rows, "No notifications")
	} else {
		for _, notification := range a.Notifications {
			// Format notification with severity color and read status
			var severityColor string
			switch notification.Severity {
			case "critical":
				severityColor = "[red]CRITICAL[-]"
			case "high":
				severityColor = "[red]HIGH[-]"
			case "medium":
				severityColor = "[yellow]MEDIUM[-]"
			case "low":
				severityColor = "[blue]LOW[-]"
			default:
				severityColor = "[green]INFO[-]"
			}

			// Add read/unread indicator
			readStatus := " "
			if !notification.Read {
				readStatus = "•"
			}

			// Add action URL indicator if present
			actionIndicator := ""
			if notification.ActionURL != "" {
				actionIndicator = " → "
			}

			// Format time nicely
			timeStr := notification.Timestamp.Format("15:04:05")

			rows = append(rows, fmt.Sprintf("%s [%s] %s%s%s (%s)",
				readStatus, severityColor, notification.Title, actionIndicator,
				notification.Source, timeStr))
		}
	}

	// Update the list
	list.Rows = rows

	// If we have notifications, select the first one if none is selected
	if len(a.Notifications) > 0 {
		if a.SelectedNotification < 0 || a.SelectedNotification >= len(a.Notifications) {
			a.SelectedNotification = 0
		}

		// Update the selected row
		list.SelectedRow = a.SelectedNotification

		// Update notification details
		if a.NotificationDetailMode {
			a.updateNotificationDetails()
		}
	}

	// Update the filter display
	filterDisplay := tab.Panels[1]

	// Build filter description
	filterText := "Active filters:\n\n"

	// Check if we have any active filters
	hasSourceFilters := false
	hasSeverityFilters := false

	// This would check for active filters in the real implementation
	// For now, just display the available filters

	if !hasSourceFilters && !hasSeverityFilters {
		filterText = "No active filters. Press 'f' to add filters."
	}

	filterDisplay.Text = filterText
}

// updateNotificationDetails updates the details view for the selected notification
func (a *App) updateNotificationDetails() {
	// Get the notifications tab
	tab := a.getTabByName("Notifications")
	if tab == nil || len(tab.Panels) < 1 {
		return
	}

	// Get the details panel
	details := tab.Panels[0]

	// Check if we have notifications and a valid selection
	if len(a.Notifications) > 0 && a.SelectedNotification < len(a.Notifications) {
		notification := a.Notifications[a.SelectedNotification]

		// Format severity with color
		var severityColor string
		switch notification.Severity {
		case "critical":
			severityColor = "[red]CRITICAL[-]"
		case "high":
			severityColor = "[red]HIGH[-]"
		case "medium":
			severityColor = "[yellow]MEDIUM[-]"
		case "low":
			severityColor = "[blue]LOW[-]"
		default:
			severityColor = "[green]INFO[-]"
		}

		// Format notification details
		detailsText := fmt.Sprintf(`
Title: %s
Severity: %s
Source: %s
Time: %s
Read: %v

Message:
%s
`,
			notification.Title,
			severityColor,
			notification.Source,
			notification.Timestamp.Format("2006-01-02 15:04:05"),
			notification.Read,
			notification.Message)

		// Add action URL if present
		if notification.ActionURL != "" {
			detailsText += fmt.Sprintf("\nAction URL: %s (press 'o' to open)", notification.ActionURL)
		}

		details.Text = detailsText
	} else {
		details.Text = "No notification selected"
	}
}

// updateTabBadges updates the tab badges for unread notifications
func (a *App) updateTabBadges() {
	// Skip if storage is not available
	if a.Storage == nil {
		return
	}

	// Get unread notification count
	unreadCount, err := a.Storage.GetUnreadNotificationCount()
	if err != nil {
		return
	}

	// Update notifications tab badge
	notificationsTab := a.getTabByName("Notifications")
	if notificationsTab != nil {
		notificationsTab.HasUnread = unreadCount > 0
	}

	// Update tab names to show/hide badges
	a.updateTabNames()
}

// addNewAnnotation prepares the UI for adding a new annotation
func (a *App) addNewAnnotation() {
	a.AddingAnnotation = true

	// Create annotation form if not exists
	if a.AnnotationForm == nil {
		a.AnnotationForm = widgets.NewParagraph()
		a.AnnotationForm.Title = "Add Annotation"
		a.AnnotationForm.SetRect(
			a.TermWidth/4,
			a.TermHeight/4,
			a.TermWidth*3/4,
			a.TermHeight*3/4)
	}

	// Set initial form state
	a.AnnotationForm.Text = "Add Annotation\n\nTitle: █\n\nDescription: \n\nPress ENTER to continue, ESC to cancel"
	a.StatusBar.Text = "Enter annotation title:"

	// Render the form
	ui.Render(a.AnnotationForm)
}

// submitAnnotation creates and saves a new annotation
func (a *App) submitAnnotation(title, description string) {
	// Create a new annotation
	annotation := models.EventAnnotation{
		ID:          fmt.Sprintf("ann-%d", time.Now().UnixNano()),
		Title:       title,
		Description: description,
		Timestamp:   time.Now(),
		Type:        "manual",
		Tags:        []string{"user-created"},
	}

	// Save to storage if available
	if a.Storage != nil {
		if err := a.Storage.AddEventAnnotation(annotation); err != nil {
			a.StatusBar.Text = fmt.Sprintf("Error saving annotation: %s", err.Error())
			return
		}

		// Update cached annotations
		annotations, err := a.Storage.GetEventAnnotations(a.HistoryRange)
		if err == nil {
			a.Annotations = annotations
		}

		a.StatusBar.Text = "Annotation added successfully"
	} else {
		a.StatusBar.Text = "Storage not available, annotation not saved"
	}
}
