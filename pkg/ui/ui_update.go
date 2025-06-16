package ui

import (
	"fmt"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// Variable to hold history range options
var historyRangeOptions = []struct {
	label string
	value time.Duration
}{
	{"1h", 1 * time.Hour},
	{"6h", 6 * time.Hour},
	{"12h", 12 * time.Hour},
	{"24h", 24 * time.Hour},
	{"3d", 72 * time.Hour},
	{"7d", 168 * time.Hour},
}

// updateData refreshes all data from collectors
func (a *App) updateData() {
	// Update system metrics tab
	a.updateSystemTabData()

	// Update HTTP tab
	a.updateHTTPTabData()

	// Update Git tab
	a.updateGitTabData()

	// Update Cloud tab
	a.updateCloudTabData()

	// Update Kubernetes tab
	a.updateKubernetesTabData()

	// Update CI/CD tab
	a.updateCICDTabData()

	// Update History tab
	a.updateHistoryTabData()

	// Update Notifications tab
	a.updateNotificationsTabData()

	// Update Plugins tab
	a.updatePluginsTabData()
}

// updateSystemTabData updates the System tab with current metrics
func (a *App) updateSystemTabData() {
	// Get the system tab
	tab := a.getTabByName("System")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create CPU gauge
		cpuGauge := widgets.NewGauge()
		cpuGauge.Title = "CPU Usage"
		cpuGauge.BarColor = ui.ColorRed
		cpuGauge.BorderStyle.Fg = ui.ColorCyan

		// Create memory gauge
		memGauge := widgets.NewGauge()
		memGauge.Title = "Memory Usage"
		memGauge.BarColor = ui.ColorBlue
		memGauge.BorderStyle.Fg = ui.ColorCyan

		// Create CPU sparkline
		cpuSparkline := widgets.NewSparkline()
		cpuSparkline.Title = "CPU History"
		cpuSparkline.LineColor = ui.ColorRed
		cpuSparklineGroup := widgets.NewSparklineGroup(cpuSparkline)
		cpuSparklineGroup.Title = "CPU History"
		cpuSparklineGroup.BorderStyle.Fg = ui.ColorCyan

		// Create disk usage chart
		diskChart := widgets.NewBarChart()
		diskChart.Title = "Disk Usage"
		diskChart.BarWidth = 5
		diskChart.BarColors = []ui.Color{ui.ColorGreen, ui.ColorYellow, ui.ColorRed}
		diskChart.NumFormatter = func(f float64) string {
			return fmt.Sprintf("%.1f%%", f)
		}
		diskChart.BorderStyle.Fg = ui.ColorCyan

		// Create process table
		processTable := widgets.NewTable()
		processTable.Title = "Processes"
		processTable.RowSeparator = true
		processTable.BorderStyle.Fg = ui.ColorCyan
		processTable.ColumnWidths = []int{10, 8, 12, 60}
		processTable.Rows = [][]string{{"PID", "CPU%", "Memory", "Command"}} // Header as first row
		processTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		processTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)} // Header style

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{cpuGauge, memGauge, cpuSparklineGroup, diskChart, processTable}
		tab.Gauges = []*widgets.Gauge{cpuGauge, memGauge}
		tab.Sparklines = []*widgets.SparklineGroup{cpuSparklineGroup}
		tab.BarCharts = []*widgets.BarChart{diskChart}
		tab.Tables = []*widgets.Table{processTable}
	}

	// Update widget data if collector is available
	if a.SystemCollector != nil {
		metrics := a.SystemCollector.GetLatestMetrics()

		// Update CPU gauge
		if len(tab.Gauges) > 0 {
			cpuGauge := tab.Gauges[0]
			cpuGauge.Percent = int(metrics.CPU.UsagePercent)
			cpuGauge.Label = fmt.Sprintf("%.1f%%", metrics.CPU.UsagePercent)
		}

		// Update memory gauge
		if len(tab.Gauges) > 1 {
			memGauge := tab.Gauges[1]
			memGauge.Percent = int(metrics.Memory.UsagePercent)
			memGauge.Label = fmt.Sprintf("%.1f%% (%.1f GB / %.1f GB)",
				metrics.Memory.UsagePercent,
				float64(metrics.Memory.Used)/(1024*1024*1024),
				float64(metrics.Memory.Total)/(1024*1024*1024))
		}

		// Update CPU sparkline
		if len(tab.Sparklines) > 0 {
			cpuSparklineGroup := tab.Sparklines[0]
			if len(cpuSparklineGroup.Sparklines) > 0 {
				cpuSparkline := cpuSparklineGroup.Sparklines[0]
				// Add current CPU usage to the sparkline data
				if len(cpuSparkline.Data) >= 100 {
					// Limit to 100 points
					cpuSparkline.Data = append(cpuSparkline.Data[1:], float64(metrics.CPU.UsagePercent))
				} else {
					cpuSparkline.Data = append(cpuSparkline.Data, float64(metrics.CPU.UsagePercent))
				}
			}
		}

		// Update disk chart
		if len(tab.BarCharts) > 0 {
			diskChart := tab.BarCharts[0]
			diskData := make([]float64, len(metrics.Disk.Filesystems))
			diskLabels := make([]string, len(metrics.Disk.Filesystems))

			for i, disk := range metrics.Disk.Filesystems {
				diskData[i] = disk.UsagePercent
				diskLabels[i] = disk.MountPoint
				// Truncate long mount points
				if len(diskLabels[i]) > 15 {
					diskLabels[i] = "..." + diskLabels[i][len(diskLabels[i])-12:]
				}
			}

			diskChart.Data = diskData
			diskChart.Labels = diskLabels
		}

		// Update process table (simplified since TopProcesses doesn't exist in the model)
		if len(tab.Tables) > 0 {
			processTable := tab.Tables[0]
			rows := [][]string{{"PID", "CPU%", "Memory", "Command"}} // Header

			// Add some sample data since TopProcesses field doesn't exist
			rows = append(rows, []string{
				"1234",
				"5.2%",
				"128 MB",
				"sample-process",
			})

			processTable.Rows = rows
		}
	}
}

// updateHTTPTabData updates the HTTP tab with current metrics
func (a *App) updateHTTPTabData() {
	// Get the HTTP tab
	tab := a.getTabByName("HTTP")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.widgets) == 0 {
		// Create endpoints table
		endpointsTable := widgets.NewTable()
		endpointsTable.Title = "HTTP Endpoints"
		endpointsTable.RowSeparator = true
		endpointsTable.BorderStyle.Fg = ui.ColorCyan
		endpointsTable.ColumnWidths = []int{30, 15, 12, 14, 20}
		endpointsTable.Header = []string{"Endpoint", "Status", "Response Time", "Availability", "Last Check"}
		endpointsTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		endpointsTable.HeaderStyle = ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)

		// Create response time sparkline
		respTimeSparkline := widgets.NewSparkline()
		respTimeSparkline.LineColor = ui.ColorGreen
		respTimeSparklineGroup := widgets.NewSparklineGroup(respTimeSparkline)
		respTimeSparklineGroup.Title = "Response Time History"
		respTimeSparklineGroup.BorderStyle.Fg = ui.ColorCyan

		// Create availability sparkline
		availabilitySparkline := widgets.NewSparkline()
		availabilitySparkline.LineColor = ui.ColorBlue
		availabilitySparklineGroup := widgets.NewSparklineGroup(availabilitySparkline)
		availabilitySparklineGroup.Title = "Availability History"
		availabilitySparklineGroup.BorderStyle.Fg = ui.ColorCyan

		// Create details panel
		detailsPanel := widgets.NewParagraph()
		detailsPanel.Title = "Details"
		detailsPanel.WrapText = true
		detailsPanel.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.widgets = []ui.Drawable{endpointsTable, respTimeSparklineGroup, availabilitySparklineGroup, detailsPanel}
		tab.tables = []*widgets.Table{endpointsTable}
		tab.sparklines = []*widgets.SparklineGroup{respTimeSparklineGroup, availabilitySparklineGroup}
		tab.panels = []*widgets.Paragraph{detailsPanel}
	}

	// Update widget data if collector is available
	if a.httpCollector != nil {
		metrics := a.httpCollector.GetLatestMetrics()

		// Update endpoints table
		if len(tab.tables) > 0 {
			endpointsTable := tab.tables[0]
			rows := [][]string{}

			for _, endpoint := range metrics.Endpoints {
				// Determine status color based on status code
				statusStr := "-"
				if endpoint.StatusCode > 0 {
					statusStr = fmt.Sprintf("%d", endpoint.StatusCode)
				}

				// Add color based on status
				var statusStyled string
				if endpoint.StatusCode >= 200 && endpoint.StatusCode < 300 {
					statusStyled = fmt.Sprintf("[green]%s[-]", statusStr)
				} else if endpoint.StatusCode >= 300 && endpoint.StatusCode < 400 {
					statusStyled = fmt.Sprintf("[blue]%s[-]", statusStr)
				} else if endpoint.StatusCode >= 400 && endpoint.StatusCode < 500 {
					statusStyled = fmt.Sprintf("[yellow]%s[-]", statusStr)
				} else if endpoint.StatusCode >= 500 {
					statusStyled = fmt.Sprintf("[red]%s[-]", statusStr)
				} else if endpoint.Error != "" {
					statusStyled = fmt.Sprintf("[red]Error[-]")
				} else {
					statusStyled = fmt.Sprintf("[gray]%s[-]", statusStr)
				}

				// Format availability as percentage
				availabilityStr := fmt.Sprintf("%.1f%%", endpoint.Availability*100)

				// Format response time
				respTimeStr := "-"
				if endpoint.ResponseTime > 0 {
					respTimeStr = fmt.Sprintf("%.2f ms", endpoint.ResponseTime)
				}

				// Format time
				timeStr := "-"
				if !endpoint.LastCheck.IsZero() {
					timeStr = formatTime(endpoint.LastCheck)
				}

				rows = append(rows, []string{
					endpoint.URL,
					statusStyled,
					respTimeStr,
					availabilityStr,
					timeStr,
				})
			}

			endpointsTable.Rows = rows
		}

		// Update response time sparkline with data for the first endpoint
		if len(metrics.Endpoints) > 0 && len(tab.sparklines) > 0 {
			respTimeSparklineGroup := tab.sparklines[0]
			if len(respTimeSparklineGroup.Sparklines) > 0 {
				respTimeSparkline := respTimeSparklineGroup.Sparklines[0]

				// Update title with endpoint name
				respTimeSparklineGroup.Title = fmt.Sprintf("Response Time: %s", metrics.Endpoints[0].URL)

				// Add current response time to sparkline data
				if len(respTimeSparkline.Data) >= 100 {
					// Limit to 100 points
					respTimeSparkline.Data = append(respTimeSparkline.Data[1:], metrics.Endpoints[0].ResponseTime)
				} else {
					respTimeSparkline.Data = append(respTimeSparkline.Data, metrics.Endpoints[0].ResponseTime)
				}
			}
		}

		// Update availability sparkline with data for the first endpoint
		if len(metrics.Endpoints) > 0 && len(tab.sparklines) > 1 {
			availabilitySparklineGroup := tab.sparklines[1]
			if len(availabilitySparklineGroup.Sparklines) > 0 {
				availabilitySparkline := availabilitySparklineGroup.Sparklines[0]

				// Update title with endpoint name
				availabilitySparklineGroup.Title = fmt.Sprintf("Availability: %s", metrics.Endpoints[0].URL)

				// Add current availability to sparkline data (multiply by 100 for percentage)
				if len(availabilitySparkline.Data) >= 100 {
					// Limit to 100 points
					availabilitySparkline.Data = append(availabilitySparkline.Data[1:], metrics.Endpoints[0].Availability*100)
				} else {
					availabilitySparkline.Data = append(availabilitySparkline.Data, metrics.Endpoints[0].Availability*100)
				}
			}
		}

		// Update details panel with information about the first endpoint
		if len(metrics.Endpoints) > 0 && len(tab.panels) > 0 {
			detailsPanel := tab.panels[0]
			endpoint := metrics.Endpoints[0]

			// Format details text
			detailsText := fmt.Sprintf(`
URL: %s
Method: %s
Last Status: %d
Response Time: %.2f ms
Availability: %.1f%%
Last Check: %s
`,
				endpoint.URL,
				endpoint.Method,
				endpoint.StatusCode,
				endpoint.ResponseTime,
				endpoint.Availability*100,
				formatTime(endpoint.LastCheck))

			// Add error if present
			if endpoint.Error != "" {
				detailsText += fmt.Sprintf("\nError: %s", endpoint.Error)
			}

			detailsPanel.Text = detailsText
		}
	}
}

// updateGitTabData updates the Git tab with current status
func (a *App) updateGitTabData() {
	// Get the Git tab
	tab := a.getTabByName("Git")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.widgets) == 0 {
		// Create repository status paragraph
		repoStatus := widgets.NewParagraph()
		repoStatus.Title = "Repository Status"
		repoStatus.WrapText = true
		repoStatus.BorderStyle.Fg = ui.ColorCyan

		// Create changes table
		changesTable := widgets.NewTable()
		changesTable.Title = "Changed Files"
		changesTable.RowSeparator = true
		changesTable.BorderStyle.Fg = ui.ColorCyan
		changesTable.ColumnWidths = []int{10, 80}
		changesTable.Header = []string{"Status", "File"}
		changesTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		changesTable.HeaderStyle = ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)

		// Create commit history table
		commitTable := widgets.NewTable()
		commitTable.Title = "Recent Commits"
		commitTable.RowSeparator = true
		commitTable.BorderStyle.Fg = ui.ColorCyan
		commitTable.ColumnWidths = []int{8, 20, 60}
		commitTable.Header = []string{"Hash", "Author", "Message"}
		commitTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		commitTable.HeaderStyle = ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)

		// Create branch list
		branchList := widgets.NewList()
		branchList.Title = "Branches"
		branchList.WrapText = false
		branchList.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.widgets = []ui.Drawable{repoStatus, changesTable, commitTable, branchList}
		tab.panels = []*widgets.Paragraph{repoStatus}
		tab.tables = []*widgets.Table{changesTable, commitTable}
		tab.lists = []*widgets.List{branchList}
	}

	// Update widget data if collector is available
	if a.gitCollector != nil {
		metrics := a.gitCollector.GetLatestMetrics()

		// Update repository status
		if len(tab.panels) > 0 {
			repoStatus := tab.panels[0]

			// Only update if we have valid repo info
			if metrics.RepoPath != "" {
				// Format status text
				statusText := fmt.Sprintf(`
Repository: %s
Current Branch: %s
Remote: %s
Commits Ahead: %d
Commits Behind: %d
Modified Files: %d
Staged Files: %d
Untracked Files: %d
`,
					metrics.RepoPath,
					metrics.CurrentBranch,
					metrics.RemoteURL,
					metrics.CommitsAhead,
					metrics.CommitsBehind,
					metrics.ModifiedFiles,
					metrics.StagedFiles,
					metrics.UntrackedFiles)

				repoStatus.Text = statusText
			} else {
				repoStatus.Text = "No Git repository configured"
			}
		}

		// Update changes table
		if len(tab.tables) > 0 && len(metrics.ChangedFiles) > 0 {
			changesTable := tab.tables[0]
			rows := [][]string{}

			for _, file := range metrics.ChangedFiles {
				// Color-code status
				var statusStyled string
				switch file.Status {
				case "modified":
					statusStyled = "[yellow]Modified[-]"
				case "added":
					statusStyled = "[green]Added[-]"
				case "deleted":
					statusStyled = "[red]Deleted[-]"
				case "renamed":
					statusStyled = "[blue]Renamed[-]"
				case "untracked":
					statusStyled = "[gray]Untracked[-]"
				default:
					statusStyled = file.Status
				}

				rows = append(rows, []string{
					statusStyled,
					file.Path,
				})
			}

			changesTable.Rows = rows
		}

		// Update commit history
		if len(tab.tables) > 1 && len(metrics.RecentCommits) > 0 {
			commitTable := tab.tables[1]
			rows := [][]string{}

			for _, commit := range metrics.RecentCommits {
				// Truncate commit hash
				shortHash := commit.Hash
				if len(shortHash) > 8 {
					shortHash = shortHash[:8]
				}

				// Truncate commit message if too long
				commitMsg := commit.Message
				if len(commitMsg) > 57 {
					commitMsg = commitMsg[:54] + "..."
				}

				rows = append(rows, []string{
					shortHash,
					commit.Author,
					commitMsg,
				})
			}

			commitTable.Rows = rows
		}

		// Update branch list
		if len(tab.lists) > 0 && len(metrics.Branches) > 0 {
			branchList := tab.lists[0]
			rows := []string{}

			for _, branch := range metrics.Branches {
				// Mark current branch
				if branch == metrics.CurrentBranch {
					rows = append(rows, fmt.Sprintf("[green]* %s[-]", branch))
				} else {
					rows = append(rows, fmt.Sprintf("  %s", branch))
				}
			}

			branchList.Rows = rows
		}
	}
}

// updateCloudTabData updates the Cloud tab with current metrics
func (a *App) updateCloudTabData() {
	// Get the cloud tab
	tab := a.getTabByName("Cloud")
	if tab == nil {
		return
	}

	// Basic implementation - would be expanded based on actual cloud metrics
	if a.CloudCollector != nil {
		// Update cloud metrics if collector is available
		// This would be implemented based on the actual cloud collector interface
	}
}

// updateKubernetesTabData updates the Kubernetes tab with current metrics
func (a *App) updateKubernetesTabData() {
	// Get the kubernetes tab
	tab := a.getTabByName("Kubernetes")
	if tab == nil {
		return
	}

	// Basic implementation - would be expanded based on actual Kubernetes metrics
	if a.KubernetesCollector != nil {
		// Update Kubernetes metrics if collector is available
		// This would be implemented based on the actual Kubernetes collector interface
	}
}

// updateCICDTabData updates the CI/CD tab with current metrics
func (a *App) updateCICDTabData() {
	// Get the CI/CD tab
	tab := a.getTabByName("CI/CD")
	if tab == nil {
		return
	}

	// Basic implementation - would be expanded based on actual CI/CD metrics
	if a.CICDCollector != nil {
		// Update CI/CD metrics if collector is available
		// This would be implemented based on the actual CI/CD collector interface
	}
}

// updateHistoryTabData updates the History tab with current metrics
func (a *App) updateHistoryTabData() {
	// Get the history tab
	tab := a.getTabByName("History")
	if tab == nil {
		return
	}

	// Basic implementation - would be expanded based on actual history data
	// This would load historical data from storage and create plots
}

// updateLayout updates the terminal UI layout
func (a *App) updateLayout() {
	// Calculate terminal dimensions
	termWidth, termHeight := ui.TerminalDimensions()
	a.termWidth = termWidth
	a.termHeight = termHeight

	// Create grid if not exists
	if a.grid == nil {
		a.grid = ui.NewGrid()
	}

	// Create status bar if not exists
	if a.statusBar == nil {
		a.statusBar = widgets.NewParagraph()
		a.statusBar.Text = "Ready"
		a.statusBar.BorderStyle.Fg = ui.ColorCyan
	}

	// Create tab bar if not exists
	if a.tabBar == nil {
		a.tabBar = widgets.NewTabPane(a.getTabNames()...)
		a.tabBar.ActiveTabStyle = ui.NewStyle(ui.ColorBlack, ui.ColorCyan)
		a.tabBar.PaddingLeft = 1
		a.tabBar.PaddingRight = 1
		a.tabBar.BorderStyle.Fg = ui.ColorCyan
	}

	// Create help panel if not exists
	if a.helpPanel == nil {
		a.helpPanel = widgets.NewParagraph()
		a.helpPanel.Title = "Help"
		a.helpPanel.BorderStyle.Fg = ui.ColorCyan
		a.helpPanel.Text = `
Global:
  q, Ctrl+C: Quit
  Tab, l, n, →: Next tab
  Shift+Tab, h, p, ←: Previous tab
  1-9: Switch to tab by number
  r: Refresh data
  ?: Show/hide help

Charts and Metrics:
  z: Enter zoom mode
  c: Toggle comparison mode
  [, ]: Adjust history time range

Notifications:
  f: Filter notifications
  d: View notification details
  m: Mark as read
  D: Dismiss notification
  C: Clear all notifications
  o: Open notification URL

History:
  a: Toggle annotations
  n: Add new annotation
  e: Export metrics to CSV

Press ? to hide help
`
	}

	// Set tab bar active tab
	a.tabBar.ActiveTabIndex = a.activeTabIndex

	// Set up grid layout
	a.grid.SetRect(0, 0, termWidth, termHeight)

	// Create a layout with tabbed interface
	mainHeight := termHeight - 4 // Reserve 3 for statusbar, 1 for tab bar
	mainRect := ui.NewRect(0, 3, termWidth, mainHeight+3)

	// Set up status bar at the bottom
	a.statusBar.SetRect(0, termHeight-3, termWidth, termHeight)

	// Set up tab bar at the top
	a.tabBar.SetRect(0, 0, termWidth, 3)

	// If help is visible, adjust layout
	if a.showHelp {
		helpWidth := 60
		helpHeight := 20
		helpX := (termWidth - helpWidth) / 2
		helpY := (termHeight - helpHeight) / 2
		a.helpPanel.SetRect(helpX, helpY, helpX+helpWidth, helpY+helpHeight)
	}

	// If adding annotation, show the form
	if a.addingAnnotation && a.annotationForm != nil {
		formWidth := termWidth / 2
		formHeight := termHeight / 2
		formX := (termWidth - formWidth) / 2
		formY := (termHeight - formHeight) / 2
		a.annotationForm.SetRect(formX, formY, formX+formWidth, formY+formHeight)
	}

	// Set current tab layout
	if a.activeTabIndex < len(a.tabs) {
		currentTab := a.tabs[a.activeTabIndex]
		a.layoutTab(currentTab, mainRect)
	}

	// Render the UI
	ui.Render(a.grid, a.statusBar, a.tabBar)

	// If zoom mode is active, update the zoom indicator
	if a.zoomMode {
		a.updateZoomIndicator()
	}

	// If help is visible, render it
	if a.showHelp {
		ui.Render(a.helpPanel)
	}

	// If adding annotation, render the form
	if a.addingAnnotation && a.annotationForm != nil {
		ui.Render(a.annotationForm)
	}
}

// updateTabNames updates the tab names to include badges for unread notifications
func (a *App) updateTabNames() {
	// Create a list of tab names, updating the Notifications tab if needed
	tabNames := make([]string, len(a.tabs))

	for i, tab := range a.tabs {
		if tab.hasUnread {
			// Add a badge indicator for tabs with unread notifications
			tabNames[i] = fmt.Sprintf("●%s", tab.name)
		} else {
			tabNames[i] = tab.name
		}
	}

	// Update tab bar names
	a.tabBar.TabNames = tabNames
}

// updateHistoryRange updates the history range based on the current index
func (a *App) updateHistoryRange() {
	if a.historyRangeIdx >= 0 && a.historyRangeIdx < len(historyRangeOptions) {
		a.historyRange = historyRangeOptions[a.historyRangeIdx].value
		a.statusBar.Text = fmt.Sprintf("History range set to %s",
			historyRangeOptions[a.historyRangeIdx].label)

		// If we have storage, update the annotations for the new time range
		if a.storage != nil {
			annotations, err := a.storage.GetEventAnnotations(a.historyRange)
			if err == nil {
				a.annotations = annotations
			}
		}
	}
}
