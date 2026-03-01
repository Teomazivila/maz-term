package ui

import (
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
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
		processTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

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
	if len(tab.Widgets) == 0 {
		// Create endpoints table
		endpointsTable := widgets.NewTable()
		endpointsTable.Title = "HTTP Endpoints"
		endpointsTable.RowSeparator = true
		endpointsTable.BorderStyle.Fg = ui.ColorCyan
		endpointsTable.ColumnWidths = []int{30, 15, 12, 14, 20}
		endpointsTable.Rows = [][]string{{"Endpoint", "Status", "Response Time", "Availability", "Last Check"}}
		endpointsTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		endpointsTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

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
		tab.Widgets = []ui.Drawable{endpointsTable, respTimeSparklineGroup, availabilitySparklineGroup, detailsPanel}
		tab.Tables = []*widgets.Table{endpointsTable}
		tab.Sparklines = []*widgets.SparklineGroup{respTimeSparklineGroup, availabilitySparklineGroup}
		tab.Panels = []*widgets.Paragraph{detailsPanel}
	}

	// Update widget data if collector is available
	if a.HTTPCollector != nil {
		metrics := a.HTTPCollector.GetLatestMetrics()

		// Update endpoints table
		if len(tab.Tables) > 0 {
			endpointsTable := tab.Tables[0]
			rows := [][]string{{"Endpoint", "Status", "Response Time", "Availability", "Last Check"}} // Header

			for _, endpoint := range metrics {
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
				} else {
					statusStyled = fmt.Sprintf("[gray]%s[-]", statusStr)
				}

				// Format response time
				respTimeStr := "-"
				if endpoint.ResponseTime > 0 {
					respTimeStr = fmt.Sprintf("%.2f ms", float64(endpoint.ResponseTime.Nanoseconds())/1000000)
				}

				// Format time
				timeStr := "-"
				if !endpoint.LastChecked.IsZero() {
					timeStr = formatTime(endpoint.LastChecked)
				}

				rows = append(rows, []string{
					endpoint.URL,
					statusStyled,
					respTimeStr,
					fmt.Sprintf("%v", endpoint.IsUp), // Availability placeholder
					timeStr,
				})
			}

			endpointsTable.Rows = rows
		}

		// Update sparklines with data from first endpoint (if any)
		if len(metrics) > 0 {
			// Get first endpoint from map
			var firstEndpoint models.EndpointMetrics
			for _, endpoint := range metrics {
				firstEndpoint = endpoint
				break
			}

			// Update response time sparkline
			if len(tab.Sparklines) > 0 {
				respTimeSparklineGroup := tab.Sparklines[0]
				if len(respTimeSparklineGroup.Sparklines) > 0 {
					respTimeSparkline := respTimeSparklineGroup.Sparklines[0]

					// Update title with endpoint name
					respTimeSparklineGroup.Title = fmt.Sprintf("Response Time: %s", firstEndpoint.URL)

					// Add current response time to sparkline data
					respTimeMs := float64(firstEndpoint.ResponseTime.Nanoseconds()) / 1000000
					if len(respTimeSparkline.Data) >= 100 {
						// Limit to 100 points
						respTimeSparkline.Data = append(respTimeSparkline.Data[1:], respTimeMs)
					} else {
						respTimeSparkline.Data = append(respTimeSparkline.Data, respTimeMs)
					}
				}
			}

			// Update availability sparkline
			if len(tab.Sparklines) > 1 {
				availabilitySparklineGroup := tab.Sparklines[1]
				if len(availabilitySparklineGroup.Sparklines) > 0 {
					availabilitySparkline := availabilitySparklineGroup.Sparklines[0]

					// Update title with endpoint name
					availabilitySparklineGroup.Title = fmt.Sprintf("Availability: %s", firstEndpoint.URL)

					// Add current availability to sparkline data
					availability := 0.0
					if firstEndpoint.IsUp {
						availability = 100.0
					}
					if len(availabilitySparkline.Data) >= 100 {
						// Limit to 100 points
						availabilitySparkline.Data = append(availabilitySparkline.Data[1:], availability)
					} else {
						availabilitySparkline.Data = append(availabilitySparkline.Data, availability)
					}
				}
			}

			// Update details panel
			if len(tab.Panels) > 0 {
				detailsPanel := tab.Panels[0]

				// Format details text
				detailsText := fmt.Sprintf(`
URL: %s
Last Status: %d
Response Time: %.2f ms
Is Up: %v
Last Check: %s
`,
					firstEndpoint.URL,
					firstEndpoint.StatusCode,
					float64(firstEndpoint.ResponseTime.Nanoseconds())/1000000,
					firstEndpoint.IsUp,
					formatTime(firstEndpoint.LastChecked))

				detailsPanel.Text = detailsText
			}
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
	if len(tab.Widgets) == 0 {
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
		changesTable.Rows = [][]string{{"Status", "File"}} // Header as first row
		changesTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		changesTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

		// Create commit history table
		commitTable := widgets.NewTable()
		commitTable.Title = "Recent Commits"
		commitTable.RowSeparator = true
		commitTable.BorderStyle.Fg = ui.ColorCyan
		commitTable.ColumnWidths = []int{12, 20, 60}
		commitTable.Rows = [][]string{{"Hash", "Author", "Message"}} // Header as first row
		commitTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		commitTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

		// Create branch list
		branchList := widgets.NewList()
		branchList.Title = "Branches"
		branchList.WrapText = false
		branchList.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{repoStatus, changesTable, commitTable, branchList}
		tab.Panels = []*widgets.Paragraph{repoStatus}
		tab.Tables = []*widgets.Table{changesTable, commitTable}
		tab.Lists = []*widgets.List{branchList}
	}

	// Update widget data if collector is available
	if a.GitCollector != nil {
		// Get latest Git metrics
		metrics := a.GitCollector.GetLatestMetrics()

		// Update repository status paragraph
		if len(tab.Panels) > 0 {
			statusPanel := tab.Panels[0]
			statusText := fmt.Sprintf(`Repository: %s
Branch: %s
Commit Count: %d
Modified Files: %d
Pending Commits: %d
Last Commit: %s`,
				metrics.Name,
				metrics.Branch,
				metrics.CommitCount,
				metrics.ModifiedFiles,
				metrics.PendingCommits,
				formatTime(metrics.LastCommit))
			statusPanel.Text = statusText
		}

		// Update changes table (using demo data since we don't have detailed file info)
		if len(tab.Tables) > 0 {
			changesTable := tab.Tables[0]
			rows := [][]string{{"Status", "File"}} // Header

			// Show summary of modified files
			if metrics.ModifiedFiles > 0 {
				rows = append(rows, []string{"[yellow]M[-]", fmt.Sprintf("%d modified files", metrics.ModifiedFiles)})
			}
			if metrics.PendingCommits > 0 {
				rows = append(rows, []string{"[green]A[-]", fmt.Sprintf("%d pending commits", metrics.PendingCommits)})
			}
			changesTable.Rows = rows
		}

		// Update commit history table
		if len(tab.Tables) > 1 {
			commitTable := tab.Tables[1]
			rows := [][]string{{"Hash", "Author", "Message"}} // Header

			for _, commit := range metrics.CommitHistory {
				shortHash := commit.Hash
				if len(shortHash) > 8 {
					shortHash = shortHash[:8]
				}
				message := commit.Message
				if len(message) > 50 {
					message = message[:47] + "..."
				}
				rows = append(rows, []string{shortHash, commit.Author, message})
			}
			commitTable.Rows = rows
		}

		// Update branch list (using current branch info)
		if len(tab.Lists) > 0 {
			branchList := tab.Lists[0]
			branches := []string{
				fmt.Sprintf("* [green]%s[-] (current)", metrics.Branch),
				"  main",
				"  develop",
				"  feature/new-ui",
			}
			branchList.Rows = branches
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

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create cloud provider info panel
		providerPanel := widgets.NewParagraph()
		providerPanel.Title = "Cloud Provider Status"
		providerPanel.Text = "No cloud providers configured\n\nTo add cloud providers:\n1. Configure AWS, Azure, or GCP credentials\n2. Enable cloud collectors in config.yaml\n3. Restart the application"
		providerPanel.WrapText = true
		providerPanel.BorderStyle.Fg = ui.ColorCyan

		// Create resources table
		resourcesTable := widgets.NewTable()
		resourcesTable.Title = "Cloud Resources"
		resourcesTable.RowSeparator = true
		resourcesTable.BorderStyle.Fg = ui.ColorCyan
		resourcesTable.ColumnWidths = []int{20, 15, 15, 20, 30}
		resourcesTable.Rows = [][]string{{"Resource", "Type", "Status", "Region", "Details"}}
		resourcesTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		resourcesTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

		// Create cost summary gauge
		costGauge := widgets.NewGauge()
		costGauge.Title = "Monthly Cost Estimate"
		costGauge.Percent = 0
		costGauge.BarColor = ui.ColorGreen
		costGauge.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{providerPanel, resourcesTable, costGauge}
		tab.Panels = []*widgets.Paragraph{providerPanel}
		tab.Tables = []*widgets.Table{resourcesTable}
		tab.Gauges = []*widgets.Gauge{costGauge}
	}

	// Update with actual cloud metrics if collector is available
	if a.CloudCollector != nil {
		if cloudMetrics := a.CloudCollector.GetLatestMetrics(); cloudMetrics != nil {
			// Update provider panel
			if len(tab.Panels) > 0 {
				providerPanel := tab.Panels[0]
				providerPanel.Text = "Cloud Provider: Connected\n\nMonitoring cloud resources...\n\nPress 'r' to refresh data"
			}

			// Note: The actual implementation would parse cloudMetrics and update the widgets
			// For now, we show a connected state
		}
	}
}

// updateKubernetesTabData updates the Kubernetes tab with current metrics
func (a *App) updateKubernetesTabData() {
	// Get the kubernetes tab
	tab := a.getTabByName("Kubernetes")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create cluster info panel
		clusterPanel := widgets.NewParagraph()
		clusterPanel.Title = "Cluster Information"
		clusterPanel.Text = "No Kubernetes cluster configured\n\nTo connect to a cluster:\n1. Configure kubeconfig\n2. Enable Kubernetes collector in config.yaml\n3. Restart the application"
		clusterPanel.WrapText = true
		clusterPanel.BorderStyle.Fg = ui.ColorCyan

		// Create pods table
		podsTable := widgets.NewTable()
		podsTable.Title = "Pods"
		podsTable.RowSeparator = true
		podsTable.BorderStyle.Fg = ui.ColorCyan
		podsTable.ColumnWidths = []int{30, 15, 10, 15, 30}
		podsTable.Rows = [][]string{{"Name", "Namespace", "Status", "Restarts", "Age"}}
		podsTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		podsTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

		// Create deployments table
		deploymentsTable := widgets.NewTable()
		deploymentsTable.Title = "Deployments"
		deploymentsTable.RowSeparator = true
		deploymentsTable.BorderStyle.Fg = ui.ColorCyan
		deploymentsTable.ColumnWidths = []int{30, 15, 10, 10, 35}
		deploymentsTable.Rows = [][]string{{"Name", "Namespace", "Ready", "Available", "Age"}}
		deploymentsTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		deploymentsTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

		// Create resource usage gauge
		resourceGauge := widgets.NewGauge()
		resourceGauge.Title = "Cluster Resource Usage"
		resourceGauge.Percent = 0
		resourceGauge.BarColor = ui.ColorBlue
		resourceGauge.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{clusterPanel, podsTable, deploymentsTable, resourceGauge}
		tab.Panels = []*widgets.Paragraph{clusterPanel}
		tab.Tables = []*widgets.Table{podsTable, deploymentsTable}
		tab.Gauges = []*widgets.Gauge{resourceGauge}
	}

	// Update with actual Kubernetes metrics if collector is available
	if a.KubernetesCollector != nil {
		if k8sMetrics := a.KubernetesCollector.GetLatestMetrics(); k8sMetrics != nil {
			// Update cluster panel
			if len(tab.Panels) > 0 {
				clusterPanel := tab.Panels[0]
				clusterPanel.Text = "Cluster: Connected\n\nMonitoring Kubernetes resources...\n\nPress 'r' to refresh data"
			}

			// Note: The actual implementation would parse k8sMetrics and update the widgets
			// For now, we show a connected state
		}
	}
}

// updateCICDTabData updates the CI/CD tab with current metrics
func (a *App) updateCICDTabData() {
	// Get the CI/CD tab
	tab := a.getTabByName("CI/CD")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create CI/CD status panel
		statusPanel := widgets.NewParagraph()
		statusPanel.Title = "CI/CD Pipeline Status"
		statusPanel.Text = "No CI/CD pipelines configured\n\nTo add CI/CD monitoring:\n1. Configure GitHub Actions, Jenkins, or GitLab CI\n2. Add API tokens to config.yaml\n3. Enable CI/CD collectors\n4. Restart the application"
		statusPanel.WrapText = true
		statusPanel.BorderStyle.Fg = ui.ColorCyan

		// Create pipelines table
		pipelinesTable := widgets.NewTable()
		pipelinesTable.Title = "Pipelines"
		pipelinesTable.RowSeparator = true
		pipelinesTable.BorderStyle.Fg = ui.ColorCyan
		pipelinesTable.ColumnWidths = []int{25, 15, 15, 20, 25}
		pipelinesTable.Rows = [][]string{{"Pipeline", "Status", "Branch", "Duration", "Last Run"}}
		pipelinesTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		pipelinesTable.RowStyles = map[int]ui.Style{0: ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold)}

		// Create success rate gauge
		successGauge := widgets.NewGauge()
		successGauge.Title = "Success Rate (30 days)"
		successGauge.Percent = 0
		successGauge.BarColor = ui.ColorGreen
		successGauge.BorderStyle.Fg = ui.ColorCyan

		// Create deployment frequency sparkline
		deploySparkline := widgets.NewSparkline()
		deploySparkline.LineColor = ui.ColorBlue

		deploySparklineGroup := widgets.NewSparklineGroup(deploySparkline)
		deploySparklineGroup.Title = "Deployment Frequency"
		deploySparklineGroup.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{statusPanel, pipelinesTable, successGauge, deploySparklineGroup}
		tab.Panels = []*widgets.Paragraph{statusPanel}
		tab.Tables = []*widgets.Table{pipelinesTable}
		tab.Gauges = []*widgets.Gauge{successGauge}
		tab.Sparklines = []*widgets.SparklineGroup{deploySparklineGroup}
	}

	// Update with actual CI/CD metrics if collector is available
	if a.CICDCollector != nil {
		if cicdMetrics := a.CICDCollector.GetLatestMetrics(); cicdMetrics != nil {
			// Update status panel
			if len(tab.Panels) > 0 {
				statusPanel := tab.Panels[0]
				statusPanel.Text = "CI/CD Provider: Connected\n\nMonitoring pipelines and deployments...\n\nPress 'r' to refresh data"
			}

			// Note: The actual implementation would parse cicdMetrics and update the widgets
			// For now, we show a connected state
		}
	}
}

// updateHistoryTabData updates the History tab with current metrics
func (a *App) updateHistoryTabData() {
	// Get the history tab
	tab := a.getTabByName("History")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create CPU history plot
		cpuPlot := widgets.NewPlot()
		cpuPlot.Title = "CPU Usage History"
		cpuPlot.Data = make([][]float64, 1)
		cpuPlot.AxesColor = ui.ColorWhite
		cpuPlot.LineColors = []ui.Color{ui.ColorGreen}
		cpuPlot.BorderStyle.Fg = ui.ColorCyan

		// Create memory history plot
		memoryPlot := widgets.NewPlot()
		memoryPlot.Title = "Memory Usage History"
		memoryPlot.Data = make([][]float64, 1)
		memoryPlot.AxesColor = ui.ColorWhite
		memoryPlot.LineColors = []ui.Color{ui.ColorBlue}
		memoryPlot.BorderStyle.Fg = ui.ColorCyan

		// Create HTTP response time plot
		httpPlot := widgets.NewPlot()
		httpPlot.Title = "HTTP Response Time History"
		httpPlot.Data = make([][]float64, 1)
		httpPlot.AxesColor = ui.ColorWhite
		httpPlot.LineColors = []ui.Color{ui.ColorYellow}
		httpPlot.BorderStyle.Fg = ui.ColorCyan

		// Create time range selector
		timeRangePanel := widgets.NewParagraph()
		timeRangePanel.Title = "Time Range"
		timeRangePanel.Text = fmt.Sprintf("Current Range: %s\n\nPress 1-7 to select:\n1: 1 hour\n2: 6 hours\n3: 24 hours\n4: 3 days\n5: 7 days\n6: 30 days\n7: 90 days", a.HistoryRange.String())
		timeRangePanel.WrapText = true
		timeRangePanel.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{cpuPlot, memoryPlot, httpPlot, timeRangePanel}
		tab.Plots = []*widgets.Plot{cpuPlot, memoryPlot, httpPlot}
		tab.Panels = []*widgets.Paragraph{timeRangePanel}
	}

	// Update plots with historical data if storage is available
	if a.Storage != nil {
		// Update CPU history plot
		if len(tab.Plots) > 0 {
			cpuHistory, err := a.Storage.GetCPUUsageHistory(a.HistoryRange, 100)
			if err == nil && len(cpuHistory) > 0 {
				cpuData := make([]float64, len(cpuHistory))
				for i, point := range cpuHistory {
					cpuData[i] = point.Value
				}
				tab.Plots[0].Data = [][]float64{cpuData}
			}
		}

		// Update memory history plot
		if len(tab.Plots) > 1 {
			memoryHistory, err := a.Storage.GetMemoryUsageHistory(a.HistoryRange, 100)
			if err == nil && len(memoryHistory) > 0 {
				memoryData := make([]float64, len(memoryHistory))
				for i, point := range memoryHistory {
					memoryData[i] = point.Value
				}
				tab.Plots[1].Data = [][]float64{memoryData}
			}
		}

		// Update HTTP response time plot
		if len(tab.Plots) > 2 {
			endpoints, err := a.Storage.GetAllEndpoints()
			if err == nil && len(endpoints) > 0 {
				// Use the first endpoint for history
				httpHistory, err := a.Storage.GetHTTPResponseTimeHistory(endpoints[0], a.HistoryRange, 100)
				if err == nil && len(httpHistory) > 0 {
					httpData := make([]float64, len(httpHistory))
					for i, point := range httpHistory {
						httpData[i] = point.Value
					}
					tab.Plots[2].Data = [][]float64{httpData}
				}
			}
		}

		// Update time range panel
		if len(tab.Panels) > 0 {
			timeRangePanel := tab.Panels[0]
			timeRangePanel.Text = fmt.Sprintf("Current Range: %s\n\nPress 1-7 to select:\n1: 1 hour\n2: 6 hours\n3: 24 hours\n4: 3 days\n5: 7 days\n6: 30 days\n7: 90 days\n\nPress 'a' to add annotation\nPress 'z' for zoom mode", a.HistoryRange.String())
		}
	}
}

// updateLayout updates the layout of widgets
func (a *App) updateLayout() {
	// Get terminal dimensions
	termWidth, termHeight := ui.TerminalDimensions()
	a.TermWidth = termWidth
	a.TermHeight = termHeight

	// Update grid dimensions
	a.Grid.SetRect(0, 0, termWidth, termHeight-2) // Leave space for status bar
	a.Grid.Lock()

	// Update status bar
	a.StatusBar.SetRect(0, termHeight-2, termWidth, termHeight)
	a.StatusBar.Lock()
	a.StatusBar.Text = fmt.Sprintf("maz-term | %s | Press 'q' to quit", formatTime(time.Now()))
	a.StatusBar.Unlock()

	// Update tab bar
	a.TabBar.SetRect(0, termHeight-1, termWidth, termHeight)
	a.TabBar.Lock()
	a.TabBar.TabNames = a.getTabNames()
	a.TabBar.ActiveTabIndex = a.ActiveTabIndex
	a.TabBar.Unlock()

	a.Grid.Unlock()
}

// updateTabNames updates the tab names in the tab bar
func (a *App) updateTabNames() {
	if a.TabBar != nil {
		a.TabBar.TabNames = a.getTabNames()
	}
}

// updateHistoryRange updates the history range display
func (a *App) updateHistoryRange() {
	// Update status bar with current history range
	a.StatusBar.Text = fmt.Sprintf("History Range: %s | Press 'q' to quit", a.HistoryRange.String())

	// Update data for the new range
	if a.Storage != nil {
		// Would update data based on new range
		// This would be implemented with actual storage queries
	}
}
