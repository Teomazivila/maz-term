package ui

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	ui "github.com/gizak/termui/v3"
)

// formatTime formats a time.Time into a readable string
func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// handleEvent processes UI events and dispatches them appropriately
func (a *App) handleEvent(e ui.Event) {
	// If export is in progress, only allow cancel (Ctrl+C)
	if a.ExportInProgress {
		if e.ID == "<C-c>" {
			a.ExportInProgress = false
			a.StatusBar.Text = "Export cancelled"
			a.updateLayout()
		}
		return
	}

	// If adding annotation, handle text input
	if a.AddingAnnotation {
		a.handleAnnotationInput(e)
		return
	}

	// In zoom mode, handle zoom-specific controls
	if a.ZoomMode {
		a.handleZoomModeEvent(e)
		return
	}

	// In notification filter mode, handle filter-specific controls
	if a.NotificationFilterMode {
		a.handleNotificationFilterEvent(e)
		return
	}

	// In notification detail mode, handle detail-specific controls
	if a.NotificationDetailMode {
		a.handleNotificationDetailEvent(e)
		return
	}

	// If currently in plugins tab, handle plugin-specific events
	if len(a.Tabs) > a.ActiveTabIndex && a.Tabs[a.ActiveTabIndex].Name == "Plugins" {
		a.handlePluginEvents(e)
	}

	// Handle common events
	switch e.ID {
	case "q", "<C-c>":
		a.Running = false
	case "?":
		a.ShowHelp = !a.ShowHelp
		if a.ShowHelp {
			a.StatusBar.Text = "Showing help. Press ? to hide."
		} else {
			a.StatusBar.Text = "Help hidden. Press ? for help."
		}
	case "<Resize>":
		payload := e.Payload.(ui.Resize)
		a.TermWidth = payload.Width
		a.TermHeight = payload.Height
		a.StatusBar.Text = fmt.Sprintf("Terminal resized to %d x %d", a.TermWidth, a.TermHeight)
	case "<Tab>", "l", "n", "<Right>":
		a.changeTab(1)
	case "<BackTab>", "h", "p", "<Left>":
		a.changeTab(-1)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx, _ := strconv.Atoi(e.ID)
		if idx <= len(a.Tabs) {
			a.ActiveTabIndex = idx - 1
			a.StatusBar.Text = fmt.Sprintf("Switched to tab: %s", a.Tabs[a.ActiveTabIndex].Name)
		}
	case "z":
		a.enterZoomMode()
	case "a":
		if a.Tabs[a.ActiveTabIndex].Name == "History" {
			a.ShowAnnotations = !a.ShowAnnotations
			if a.ShowAnnotations {
				a.StatusBar.Text = "Showing annotations"
			} else {
				a.StatusBar.Text = "Hiding annotations"
			}
		}
	case "e":
		// Export data for current tab
		a.exportData()
	case "r":
		a.StatusBar.Text = "Refreshing data..."
		a.updateData()
	case "c":
		a.ComparisonMode = !a.ComparisonMode
		if a.ComparisonMode {
			a.StatusBar.Text = "Comparison mode enabled. Select metrics to compare."
		} else {
			a.StatusBar.Text = "Comparison mode disabled."
			a.ComparisonMetrics = nil
		}
	case "A":
		// Add new annotation (changed from "n" to avoid duplicate)
		if a.Tabs[a.ActiveTabIndex].Name == "History" {
			a.addNewAnnotation()
		}
	case "[":
		// Decrease time range
		if a.HistoryRangeIdx > 0 {
			a.HistoryRangeIdx--
			a.updateHistoryRange()
		}
	case "]":
		// Increase time range
		if a.HistoryRangeIdx < len(historyRangeOptions)-1 {
			a.HistoryRangeIdx++
			a.updateHistoryRange()
		}
	case "f":
		// Toggle notification filter mode
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" {
			a.NotificationFilterMode = !a.NotificationFilterMode
			if a.NotificationFilterMode {
				a.StatusBar.Text = "Filter mode: Use arrow keys to select filters, space to toggle, enter to apply"
			} else {
				a.StatusBar.Text = "Filter mode disabled"
			}
		}
	case "d":
		// Show notification details
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" && len(a.Notifications) > 0 {
			a.NotificationDetailMode = !a.NotificationDetailMode
			if a.NotificationDetailMode {
				a.StatusBar.Text = "Showing notification details. Press 'd' to go back."
			} else {
				a.StatusBar.Text = "Notification details hidden."
			}
		}
	case "m":
		// Mark notification as read
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" && len(a.Notifications) > 0 {
			notification := a.Notifications[a.SelectedNotification]
			if !notification.Read {
				if a.Storage != nil {
					if err := a.Storage.MarkAsRead(notification.ID); err != nil {
						a.StatusBar.Text = fmt.Sprintf("Error marking notification as read: %s", err.Error())
					} else {
						a.StatusBar.Text = "Notification marked as read"
						// Refresh notifications
						a.loadNotifications()
					}
				}
			}
		}
	case "D":
		// Dismiss notification
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" && len(a.Notifications) > 0 {
			notification := a.Notifications[a.SelectedNotification]
			if a.Storage != nil {
				if err := a.Storage.DismissNotification(notification.ID); err != nil {
					a.StatusBar.Text = fmt.Sprintf("Error dismissing notification: %s", err.Error())
				} else {
					a.StatusBar.Text = "Notification dismissed"
					// Refresh notifications
					a.loadNotifications()
				}
			}
		}
	case "C":
		// Clear all notifications
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" {
			if a.Storage != nil {
				if err := a.Storage.ClearAllNotifications(); err != nil {
					a.StatusBar.Text = fmt.Sprintf("Error clearing notifications: %s", err.Error())
				} else {
					a.StatusBar.Text = "All notifications cleared"
					// Refresh notifications
					a.loadNotifications()
				}
			}
		}
	case "o":
		// Open URL from notification if it has an action URL
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" && len(a.Notifications) > 0 {
			notification := a.Notifications[a.SelectedNotification]
			if notification.ActionURL != "" {
				a.openURL(notification.ActionURL)
			}
		}
	case "<Up>":
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" && len(a.Notifications) > 0 {
			if a.SelectedNotification > 0 {
				a.SelectedNotification--
				a.StatusBar.Text = fmt.Sprintf("Selected notification %d of %d", a.SelectedNotification+1, len(a.Notifications))
			}
		}
	case "<Down>":
		if a.Tabs[a.ActiveTabIndex].Name == "Notifications" && len(a.Notifications) > 0 {
			if a.SelectedNotification < len(a.Notifications)-1 {
				a.SelectedNotification++
				a.StatusBar.Text = fmt.Sprintf("Selected notification %d of %d", a.SelectedNotification+1, len(a.Notifications))
			}
		}
	}
}

// handleZoomModeEvent handles events while in zoom mode
func (a *App) handleZoomModeEvent(e ui.Event) {
	switch e.ID {
	case "z", "q", "<Escape>":
		a.ZoomMode = false
		a.StatusBar.Text = "Zoom mode disabled"
	case "<Left>", "h":
		// Move zoom region left
		if a.ZoomStartPercent > 0.05 {
			a.ZoomStartPercent -= 0.05
			a.ZoomEndPercent -= 0.05
			a.updateZoomIndicator()
		}
	case "<Right>", "l":
		// Move zoom region right
		if a.ZoomEndPercent < 0.95 {
			a.ZoomStartPercent += 0.05
			a.ZoomEndPercent += 0.05
			a.updateZoomIndicator()
		}
	case "<Up>", "k":
		// Decrease zoom region size (zoom in)
		if a.ZoomEndPercent-a.ZoomStartPercent > 0.1 {
			a.ZoomStartPercent += 0.05
			a.ZoomEndPercent -= 0.05
			a.updateZoomIndicator()
		}
	case "<Down>", "j":
		// Increase zoom region size (zoom out)
		if a.ZoomStartPercent > 0.05 {
			a.ZoomStartPercent -= 0.05
		}
		if a.ZoomEndPercent < 0.95 {
			a.ZoomEndPercent += 0.05
		}
		a.updateZoomIndicator()
	case "<Enter>":
		// Apply zoom
		a.applyZoom()
		a.ZoomMode = false
		a.StatusBar.Text = fmt.Sprintf("Applied zoom from %s to %s",
			formatTime(a.ZoomStartTime),
			formatTime(a.ZoomEndTime))
	}
}

// handleAnnotationInput handles text input for adding annotations
func (a *App) handleAnnotationInput(e ui.Event) {
	// Static variables to hold the form values
	staticValues := struct {
		title        string
		description  string
		currentField int
		focusChanged bool
	}{}

	switch e.ID {
	case "<Escape>":
		a.AddingAnnotation = false
		a.StatusBar.Text = "Annotation cancelled"
	case "<Enter>":
		if staticValues.currentField < 2 {
			// Move to next field
			staticValues.currentField++
			staticValues.focusChanged = true
			// Update the form display
			if staticValues.currentField == 1 {
				a.StatusBar.Text = "Enter annotation description:"
			} else {
				// Submit the annotation
				a.submitAnnotation(staticValues.title, staticValues.description)
				a.AddingAnnotation = false
			}
		}
	case "<Backspace>":
		if staticValues.currentField == 0 && len(staticValues.title) > 0 {
			staticValues.title = staticValues.title[:len(staticValues.title)-1]
		} else if staticValues.currentField == 1 && len(staticValues.description) > 0 {
			staticValues.description = staticValues.description[:len(staticValues.description)-1]
		}
	default:
		// Check if this is a single character (for text input)
		if len(e.ID) == 1 {
			if staticValues.currentField == 0 {
				staticValues.title += e.ID
			} else if staticValues.currentField == 1 {
				staticValues.description += e.ID
			}
		}
	}

	// Update the annotation form
	if a.AddingAnnotation {
		formText := "Add Annotation\n\n"

		// Title field
		formText += "Title: "
		if staticValues.currentField == 0 {
			formText += staticValues.title + "█" // Add cursor
		} else {
			formText += staticValues.title
		}

		formText += "\n\n"

		// Description field
		formText += "Description: "
		if staticValues.currentField == 1 {
			formText += staticValues.description + "█" // Add cursor
		} else {
			formText += staticValues.description
		}

		formText += "\n\n"
		formText += "Press ENTER to continue, ESC to cancel"

		a.AnnotationForm.Text = formText
		ui.Render(a.AnnotationForm)

		if staticValues.focusChanged {
			staticValues.focusChanged = false
		}
	}
}

// handleNotificationFilterEvent handles events while in notification filter mode
func (a *App) handleNotificationFilterEvent(e ui.Event) {
	// Initialize filter mode data if needed
	if a.NotificationSources == nil {
		a.loadNotificationFilters()
	}

	// Static variables to manage filter state
	staticValues := struct {
		currentSection        int // 0 = sources, 1 = severities
		selectedIndex         int
		selectedSourcesMap    map[string]bool
		selectedSeveritiesMap map[string]bool
	}{}

	if staticValues.selectedSourcesMap == nil {
		staticValues.selectedSourcesMap = make(map[string]bool)
	}
	if staticValues.selectedSeveritiesMap == nil {
		staticValues.selectedSeveritiesMap = make(map[string]bool)
	}

	switch e.ID {
	case "<Escape>":
		a.NotificationFilterMode = false
		a.StatusBar.Text = "Filter mode cancelled"
	case "<Enter>":
		// Apply the filters
		var selectedSources, selectedSeverities []string

		for source, selected := range staticValues.selectedSourcesMap {
			if selected {
				selectedSources = append(selectedSources, source)
			}
		}

		for severity, selected := range staticValues.selectedSeveritiesMap {
			if selected {
				selectedSeverities = append(selectedSeverities, severity)
			}
		}

		// Apply filters and refresh notifications
		a.StatusBar.Text = "Applying filters..."
		a.loadFilteredNotifications(selectedSources, selectedSeverities)
		a.NotificationFilterMode = false
	case "<Tab>":
		// Switch between sources and severities sections
		staticValues.currentSection = (staticValues.currentSection + 1) % 2
		staticValues.selectedIndex = 0
	case "<Up>":
		// Move selection up
		if staticValues.currentSection == 0 && staticValues.selectedIndex > 0 {
			staticValues.selectedIndex--
		} else if staticValues.currentSection == 1 && staticValues.selectedIndex > 0 {
			staticValues.selectedIndex--
		}
	case "<Down>":
		// Move selection down
		if staticValues.currentSection == 0 && staticValues.selectedIndex < len(a.NotificationSources)-1 {
			staticValues.selectedIndex++
		} else if staticValues.currentSection == 1 && staticValues.selectedIndex < len(a.NotificationSeverities)-1 {
			staticValues.selectedIndex++
		}
	case "<Space>":
		// Toggle selection
		if staticValues.currentSection == 0 && len(a.NotificationSources) > 0 {
			source := a.NotificationSources[staticValues.selectedIndex]
			staticValues.selectedSourcesMap[source] = !staticValues.selectedSourcesMap[source]
		} else if staticValues.currentSection == 1 && len(a.NotificationSeverities) > 0 {
			severity := a.NotificationSeverities[staticValues.selectedIndex]
			staticValues.selectedSeveritiesMap[severity] = !staticValues.selectedSeveritiesMap[severity]
		}
	}

	// Update the filter UI
	if a.NotificationFilterMode {
		// Your UI update logic goes here...
	}
}

// handleNotificationDetailEvent handles events in notification detail mode
func (a *App) handleNotificationDetailEvent(e ui.Event) {
	switch e.ID {
	case "d", "<Escape>":
		a.NotificationDetailMode = false
		a.StatusBar.Text = "Detail mode closed"
	case "o":
		// Open URL if available
		if len(a.Notifications) > a.SelectedNotification {
			notification := a.Notifications[a.SelectedNotification]
			if notification.ActionURL != "" {
				a.openURL(notification.ActionURL)
			}
		}
	}
}

// openURL opens a URL in the default browser
func (a *App) openURL(url string) {
	a.StatusBar.Text = fmt.Sprintf("Opening URL: %s", url)

	var cmd *exec.Cmd

	// Determine the command based on OS
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default: // Linux and others
		cmd = exec.Command("xdg-open", url)
	}

	// Run the command
	err := cmd.Start()
	if err != nil {
		a.StatusBar.Text = fmt.Sprintf("Error opening URL: %s", err.Error())
		log.Printf("Error opening URL: %v", err)
	}

	// Don't wait for the command to finish - let the browser take over
}

// changeTab changes the current tab by the specified delta
func (a *App) changeTab(delta int) {
	newIndex := a.ActiveTabIndex + delta

	// Wrap around the tab list
	if newIndex < 0 {
		newIndex = len(a.Tabs) - 1
	} else if newIndex >= len(a.Tabs) {
		newIndex = 0
	}

	a.ActiveTabIndex = newIndex
	a.StatusBar.Text = fmt.Sprintf("Switched to tab: %s", a.Tabs[a.ActiveTabIndex].Name)

	// If switching to the Notifications tab, mark as read
	if a.Tabs[a.ActiveTabIndex].Name == "Notifications" {
		a.Tabs[a.ActiveTabIndex].HasUnread = false
	}
}

// enterZoomMode enters the zoom mode for interactive chart zooming
func (a *App) enterZoomMode() {
	// Only enable zoom mode in History tab
	if a.Tabs[a.ActiveTabIndex].Name == "History" {
		a.ZoomMode = true
		a.ZoomActiveChart = 0     // Start with first chart
		a.ZoomStartPercent = 0.25 // Default to middle 50%
		a.ZoomEndPercent = 0.75
		a.StatusBar.Text = "Zoom mode: Use arrow keys to adjust region, Enter to apply, Esc to cancel"
	}
}

// updateZoomIndicator updates the zoom region indicator on the charts
func (a *App) updateZoomIndicator() {
	if !a.ZoomMode {
		return
	}

	// Calculate zoom time boundaries based on current time range
	end := time.Now()
	start := end.Add(-a.HistoryRange)

	totalDuration := end.Sub(start)
	zoomStartOffset := time.Duration(float64(totalDuration) * a.ZoomStartPercent)
	zoomEndOffset := time.Duration(float64(totalDuration) * a.ZoomEndPercent)

	a.ZoomStartTime = start.Add(zoomStartOffset)
	a.ZoomEndTime = start.Add(zoomEndOffset)

	// Update status bar with zoom range
	a.StatusBar.Text = fmt.Sprintf("Zoom: %s to %s (Use arrow keys, Enter to apply, Esc to cancel)",
		formatTime(a.ZoomStartTime),
		formatTime(a.ZoomEndTime))
}

// applyZoom applies the selected zoom region to the history charts
func (a *App) applyZoom() {
	if !a.ZoomMode {
		return
	}

	// Calculate and set a new history range based on the zoom window
	a.HistoryRange = a.ZoomEndTime.Sub(a.ZoomStartTime)

	// Set a custom history range that doesn't match the predefined options
	a.HistoryRangeIdx = -1 // Custom range

	// Force a data update to reflect the zoomed range
	a.updateData()
}

// exportData exports the current tab's data
func (a *App) exportData() {
	a.StatusBar.Text = "Export functionality not yet implemented"
}
