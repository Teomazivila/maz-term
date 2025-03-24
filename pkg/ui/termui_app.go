package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// TermUIApp represents the main terminal UI application using termui
type TermUIApp struct {
	config          *config.Config
	activeTabIndex  int
	tabs            []*TermUITab
	statusBar       *widgets.Paragraph
	grid            *ui.Grid
	tabBar          *widgets.TabPane
	running         bool
	systemCollector *collector.SystemMetricsCollector
	httpCollector   *collector.HTTPHealthChecker
	gitCollector    *collector.GitStatusCollector
	termWidth       int
	termHeight      int
}

// TermUITab represents a tab in the terminal UI
type TermUITab struct {
	name       string
	grid       *ui.Grid
	widgets    []ui.Drawable
	panels     []*widgets.Paragraph
	gauges     []*widgets.Gauge
	tables     []*widgets.Table
	sparklines []*widgets.SparklineGroup
	barCharts  []*widgets.BarChart
}

// NewTermUIApp creates a new terminal UI application
func NewTermUIApp(cfg *config.Config) *TermUIApp {
	app := &TermUIApp{
		config:         cfg,
		activeTabIndex: 0,
		tabs:           []*TermUITab{},
	}

	// Create tabs from configuration
	for _, tab := range cfg.Layout {
		// Create a new dashboard tab
		app.tabs = append(app.tabs, NewTermUITab(tab))
	}

	return app
}

// NewTermUITab creates a new terminal UI tab
func NewTermUITab(cfg config.LayoutTab) *TermUITab {
	return &TermUITab{
		name:       cfg.Name,
		widgets:    []ui.Drawable{},
		panels:     []*widgets.Paragraph{},
		gauges:     []*widgets.Gauge{},
		tables:     []*widgets.Table{},
		sparklines: []*widgets.SparklineGroup{},
		barCharts:  []*widgets.BarChart{},
	}
}

// Run starts the terminal UI application
func (a *TermUIApp) Run() error {
	// Initialize termui
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
	}
	defer ui.Close()

	// Initialize collectors
	a.initCollectors()

	// Create UI elements
	a.createUI()

	// Set up event handling
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	a.running = true
	for a.running {
		select {
		case e := <-uiEvents:
			a.handleEvent(e)
		case <-ticker.C:
			a.updateData()
			ui.Render(a.grid)
		}
	}

	return nil
}

// initCollectors initializes data collectors
func (a *TermUIApp) initCollectors() {
	appCtx := context.Background()

	// System metrics collector
	a.systemCollector = collector.NewSystemMetricsCollector()
	go a.systemCollector.Start(appCtx, 2*time.Second)

	// HTTP health checker
	// Use endpoints from configuration if available
	var endpoints []models.EndpointConfig
	if a.config != nil && len(a.config.Endpoints) > 0 {
		endpoints = a.config.Endpoints
	} else {
		// Fallback to defaults
		endpoints = []models.EndpointConfig{
			{Name: "Google", URL: "https://www.google.com", Method: "GET"},
			{Name: "GitHub", URL: "https://github.com", Method: "GET"},
			{Name: "Example", URL: "https://example.com", Method: "GET"},
		}
	}
	a.httpCollector = collector.NewHTTPHealthChecker(endpoints)
	go a.httpCollector.Start(appCtx, 5*time.Second)

	// Git status collector
	// Use git repository path from configuration if available
	gitPath := ""
	if a.config != nil && a.config.Git.Repositories != nil && len(a.config.Git.Repositories) > 0 {
		gitPath = a.config.Git.Repositories[0].Path
	}
	a.gitCollector = collector.NewGitStatusCollector(gitPath)
	go a.gitCollector.Start(appCtx, 5*time.Second)
}

// createUI creates the UI layout
func (a *TermUIApp) createUI() {
	// Get terminal dimensions
	a.termWidth, a.termHeight = ui.TerminalDimensions()

	// Create tab bar
	a.tabBar = widgets.NewTabPane(getTabNames(a.tabs)...)
	a.tabBar.Border = true
	a.tabBar.Title = "DevOps Dashboard"
	a.tabBar.ActiveTabStyle = ui.NewStyle(ui.ColorWhite, ui.ColorBlue, ui.ModifierBold)
	a.tabBar.InactiveTabStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	a.tabBar.SetRect(0, 0, a.termWidth, 3)

	// Create status bar
	a.statusBar = widgets.NewParagraph()
	a.statusBar.Text = "Status: Ready"
	a.statusBar.Border = true
	a.statusBar.SetRect(0, a.termHeight-3, a.termWidth, a.termHeight)

	// Create master grid
	a.grid = ui.NewGrid()
	a.grid.SetRect(0, 0, a.termWidth, a.termHeight)

	// Create content for each tab
	for i, tab := range a.tabs {
		a.createTabContent(tab, i)
	}

	// Update layout
	a.updateLayout()

	// Render initial UI
	ui.Render(a.grid)
}

// getTabNames extracts tab names from tab array
func getTabNames(tabs []*TermUITab) []string {
	names := make([]string, len(tabs))
	for i, tab := range tabs {
		names[i] = tab.name
	}
	return names
}

// createTabContent creates content for a specific tab
func (a *TermUIApp) createTabContent(tab *TermUITab, tabIndex int) {
	// Create different content based on tab name
	switch tabIndex {
	case 0: // System Overview
		a.createSystemTabContent(tab)
	case 1: // HTTP Endpoints
		a.createHTTPTabContent(tab)
	case 2: // Git Status
		a.createGitTabContent(tab)
	}
}

// createSystemTabContent creates content for the System tab
func (a *TermUIApp) createSystemTabContent(tab *TermUITab) {
	// Create CPU gauge
	cpuGauge := widgets.NewGauge()
	cpuGauge.Title = "CPU Usage"
	cpuGauge.Percent = 0
	cpuGauge.BarColor = ui.ColorBlue
	cpuGauge.BorderStyle.Fg = ui.ColorWhite
	cpuGauge.TitleStyle.Fg = ui.ColorCyan
	tab.gauges = append(tab.gauges, cpuGauge)
	tab.widgets = append(tab.widgets, cpuGauge)

	// Create Memory gauge
	memGauge := widgets.NewGauge()
	memGauge.Title = "Memory Usage"
	memGauge.Percent = 0
	memGauge.BarColor = ui.ColorGreen
	memGauge.BorderStyle.Fg = ui.ColorWhite
	memGauge.TitleStyle.Fg = ui.ColorCyan
	tab.gauges = append(tab.gauges, memGauge)
	tab.widgets = append(tab.widgets, memGauge)

	// Create CPU Sparklines
	cpuSparkline := widgets.NewSparkline()
	cpuSparkline.Title = "CPU"
	cpuSparkline.LineColor = ui.ColorBlue
	cpuSparkline.TitleStyle.Fg = ui.ColorWhite
	cpuSparklines := widgets.NewSparklineGroup(cpuSparkline)
	cpuSparklines.Title = "CPU History"
	tab.sparklines = append(tab.sparklines, cpuSparklines)
	tab.widgets = append(tab.widgets, cpuSparklines)

	// Create Disk Usage Bar Chart
	diskChart := widgets.NewBarChart()
	diskChart.Title = "Disk Usage"
	diskChart.Labels = []string{"Root", "Home", "Data"}
	diskChart.Data = []float64{0, 0, 0}
	diskChart.BarWidth = 5
	diskChart.BarColors = []ui.Color{ui.ColorRed, ui.ColorGreen, ui.ColorBlue}
	diskChart.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}
	diskChart.NumStyles = []ui.Style{ui.NewStyle(ui.ColorBlack)}
	tab.barCharts = append(tab.barCharts, diskChart)
	tab.widgets = append(tab.widgets, diskChart)

	// Create Processes Table
	processTable := widgets.NewTable()
	processTable.Title = "Top Processes"
	processTable.Rows = [][]string{
		{"PID", "CPU%", "MEM%", "Command"},
		{"1234", "0.5", "1.2", "system"},
		{"5678", "1.2", "3.4", "browser"},
		{"9012", "0.8", "2.5", "terminal"},
	}
	processTable.TextStyle = ui.NewStyle(ui.ColorWhite)
	processTable.RowSeparator = true
	processTable.BorderStyle = ui.NewStyle(ui.ColorGreen)
	processTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
	tab.tables = append(tab.tables, processTable)
	tab.widgets = append(tab.widgets, processTable)
}

// createHTTPTabContent creates content for the HTTP tab
func (a *TermUIApp) createHTTPTabContent(tab *TermUITab) {
	// Create HTTP status table
	httpTable := widgets.NewTable()
	httpTable.Title = "HTTP Endpoints"
	httpTable.Rows = [][]string{
		{"Endpoint", "URL", "Status", "Response Time", "Last Checked"},
		{"Google", "https://www.google.com", "UP", "120ms", "Now"},
		{"GitHub", "https://github.com", "UP", "200ms", "Now"},
		{"Example", "https://example.com", "UP", "150ms", "Now"},
	}
	httpTable.TextStyle = ui.NewStyle(ui.ColorWhite)
	httpTable.RowSeparator = true
	httpTable.BorderStyle = ui.NewStyle(ui.ColorBlue)
	httpTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

	// Color status column
	httpTable.RowStyles[1] = ui.NewStyle(ui.ColorGreen)
	httpTable.RowStyles[2] = ui.NewStyle(ui.ColorGreen)
	httpTable.RowStyles[3] = ui.NewStyle(ui.ColorGreen)

	tab.tables = append(tab.tables, httpTable)
	tab.widgets = append(tab.widgets, httpTable)

	// Create response time plot
	responsePlot := widgets.NewPlot()
	responsePlot.Title = "Response Time History"
	responsePlot.Data = make([][]float64, 3)
	responsePlot.Data[0] = []float64{1, 2, 3, 4, 5}
	responsePlot.Data[1] = []float64{1.2, 1.8, 2.5, 3.0, 3.8}
	responsePlot.Data[2] = []float64{0.8, 1.5, 2.0, 2.5, 3.2}
	responsePlot.AxesColor = ui.ColorWhite
	responsePlot.LineColors = []ui.Color{ui.ColorRed, ui.ColorYellow, ui.ColorBlue}
	responsePlot.DrawDirection = widgets.DrawLeft
	tab.widgets = append(tab.widgets, responsePlot)
}

// createGitTabContent creates content for the Git tab
func (a *TermUIApp) createGitTabContent(tab *TermUITab) {
	// Create Git status table
	gitTable := widgets.NewTable()
	gitTable.Title = "Git Repository Status"
	gitTable.Rows = [][]string{
		{"Property", "Value"},
		{"Repository", ""},
		{"Branch", ""},
		{"Commit Count", "0"},
		{"Last Commit", ""},
		{"Modified Files", "0"},
		{"Pending Commits", "0"},
	}
	gitTable.TextStyle = ui.NewStyle(ui.ColorWhite)
	gitTable.RowSeparator = true
	gitTable.BorderStyle = ui.NewStyle(ui.ColorRed)
	gitTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
	tab.tables = append(tab.tables, gitTable)
	tab.widgets = append(tab.widgets, gitTable)

	// Create commit history plot
	commitPlot := widgets.NewPlot()
	commitPlot.Title = "Commit History"
	commitPlot.Data = make([][]float64, 1)
	commitPlot.Data[0] = []float64{5, 8, 12, 4, 7}
	commitPlot.AxesColor = ui.ColorWhite
	commitPlot.LineColors = []ui.Color{ui.ColorRed}
	commitPlot.DrawDirection = widgets.DrawLeft
	tab.widgets = append(tab.widgets, commitPlot)
}

// updateLayout updates the UI layout based on terminal dimensions
func (a *TermUIApp) updateLayout() {
	// Get terminal dimensions
	width, height := ui.TerminalDimensions()
	if width != a.termWidth || height != a.termHeight {
		a.termWidth = width
		a.termHeight = height

		// Update tab bar
		a.tabBar.SetRect(0, 0, width, 3)

		// Update status bar
		a.statusBar.SetRect(0, height-3, width, height)
	}

	// Calculate content area
	contentX1 := 0
	contentY1 := 3 // Below the tab bar
	contentX2 := width
	contentY2 := height - 3 // Above the status bar

	// Create grid for active tab
	activeTab := a.tabs[a.tabBar.ActiveTabIndex]

	// Configure grid based on tab type
	switch a.tabBar.ActiveTabIndex {
	case 0: // System tab
		a.configureSystemTabGrid(activeTab, contentX1, contentY1, contentX2, contentY2)
	case 1: // HTTP tab
		a.configureHTTPTabGrid(activeTab, contentX1, contentY1, contentX2, contentY2)
	case 2: // Git tab
		a.configureGitTabGrid(activeTab, contentX1, contentY1, contentX2, contentY2)
	}

	// Update master grid
	a.grid.Items = nil

	// Create row for tab bar
	tabBarRow := ui.NewRow(0.05, ui.NewCol(1, a.tabBar))
	a.grid.Set(tabBarRow)

	// Create row for content area
	var contentRow ui.GridItem
	if activeTab.grid != nil {
		contentRow = ui.NewRow(0.9, ui.NewCol(1, activeTab.grid))
	} else if len(activeTab.widgets) > 0 {
		// Fallback to first widget
		contentRow = ui.NewRow(0.9, ui.NewCol(1, activeTab.widgets[0]))
	} else {
		// Empty content
		placeholder := widgets.NewParagraph()
		placeholder.Text = "No content available"
		contentRow = ui.NewRow(0.9, ui.NewCol(1, placeholder))
	}
	a.grid.Set(contentRow)

	// Create row for status bar
	statusBarRow := ui.NewRow(0.05, ui.NewCol(1, a.statusBar))
	a.grid.Set(statusBarRow)

	a.grid.SetRect(0, 0, width, height)
}

// configureSystemTabGrid configures the grid for the system tab
func (a *TermUIApp) configureSystemTabGrid(tab *TermUITab, x1, y1, x2, y2 int) {
	if len(tab.widgets) < 4 {
		return
	}

	// Create a grid for the system tab
	grid := ui.NewGrid()
	grid.SetRect(x1, y1, x2, y2)

	// Configure grid with 2x2 layout
	grid.Set(
		ui.NewRow(0.5,
			ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[0])), // CPU gauge
			ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[1])), // Memory gauge
		),
		ui.NewRow(0.5,
			ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[2])), // CPU sparklines
			ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[3])), // Disk chart or process table
		),
	)

	tab.grid = grid
}

// configureHTTPTabGrid configures the grid for the HTTP tab
func (a *TermUIApp) configureHTTPTabGrid(tab *TermUITab, x1, y1, x2, y2 int) {
	if len(tab.widgets) < 2 {
		return
	}

	// Create a grid for the HTTP tab
	grid := ui.NewGrid()
	grid.SetRect(x1, y1, x2, y2)

	// Configure grid with table and plot
	grid.Set(
		ui.NewRow(0.6, ui.NewCol(1.0, tab.widgets[0])), // HTTP table
		ui.NewRow(0.4, ui.NewCol(1.0, tab.widgets[1])), // Response time plot
	)

	tab.grid = grid
}

// configureGitTabGrid configures the grid for the Git tab
func (a *TermUIApp) configureGitTabGrid(tab *TermUITab, x1, y1, x2, y2 int) {
	if len(tab.widgets) < 2 {
		return
	}

	// Create a grid for the Git tab
	grid := ui.NewGrid()
	grid.SetRect(x1, y1, x2, y2)

	// Configure grid with table and plot
	grid.Set(
		ui.NewRow(0.6, ui.NewCol(1.0, tab.widgets[0])), // Git status table
		ui.NewRow(0.4, ui.NewCol(1.0, tab.widgets[1])), // Commit history plot
	)

	tab.grid = grid
}

// handleEvent handles UI events
func (a *TermUIApp) handleEvent(e ui.Event) {
	switch e.ID {
	case "q", "<C-c>":
		a.running = false
	case "<Resize>":
		payload := e.Payload.(ui.Resize)
		a.termWidth = payload.Width
		a.termHeight = payload.Height
		a.tabBar.SetRect(0, 0, payload.Width, 3)
		a.statusBar.SetRect(0, payload.Height-3, payload.Width, payload.Height)
		a.updateLayout()
	case "<Left>", "h":
		a.tabBar.FocusLeft()
		a.updateLayout()
		ui.Render(a.grid)
	case "<Right>", "l":
		a.tabBar.FocusRight()
		a.updateLayout()
		ui.Render(a.grid)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(e.ID[0] - '1')
		if idx < len(a.tabs) {
			a.tabBar.ActiveTabIndex = idx
			a.updateLayout()
			ui.Render(a.grid)
		}
	}
}

// updateData updates the data displayed in the UI
func (a *TermUIApp) updateData() {
	// Update status bar with current time
	a.statusBar.Text = fmt.Sprintf("Status: Ready | Time: %s", time.Now().Format("15:04:05"))

	// Update tab-specific data
	activeTabIdx := a.tabBar.ActiveTabIndex
	switch activeTabIdx {
	case 0:
		a.updateSystemTabData()
	case 1:
		a.updateHTTPTabData()
	case 2:
		a.updateGitTabData()
	}
}

// updateSystemTabData updates the system tab data
func (a *TermUIApp) updateSystemTabData() {
	metrics := a.systemCollector.GetLatestMetrics()
	tab := a.tabs[0]

	// CPU gauge
	if len(tab.gauges) > 0 {
		tab.gauges[0].Percent = int(metrics.CPU.UsagePercent)
		tab.gauges[0].Label = fmt.Sprintf("%.2f%%", metrics.CPU.UsagePercent)
	}

	// Memory gauge
	if len(tab.gauges) > 1 {
		tab.gauges[1].Percent = int(metrics.Memory.UsagePercent)
		tab.gauges[1].Label = fmt.Sprintf("%.2f%% (%.1f GB / %.1f GB)",
			metrics.Memory.UsagePercent,
			float64(metrics.Memory.Used)/(1024*1024*1024),
			float64(metrics.Memory.Total)/(1024*1024*1024))
	}

	// CPU sparklines
	if len(tab.sparklines) > 0 && len(tab.sparklines[0].Sparklines) > 0 {
		sparkline := tab.sparklines[0].Sparklines[0]
		if len(sparkline.Data) >= 100 {
			sparkline.Data = sparkline.Data[1:]
		}
		sparkline.Data = append(sparkline.Data, metrics.CPU.UsagePercent)
		tab.sparklines[0].Sparklines[0] = sparkline
	}

	// Disk chart
	if len(tab.barCharts) > 0 {
		barChart := tab.barCharts[0]
		barChart.Labels = []string{}
		barChart.Data = []float64{}

		for _, fs := range metrics.Disk.Filesystems {
			if len(fs.MountPoint) > 0 {
				barChart.Labels = append(barChart.Labels, fs.MountPoint)
				barChart.Data = append(barChart.Data, fs.UsagePercent)
			}
		}
	}
}

// updateHTTPTabData updates the HTTP tab data
func (a *TermUIApp) updateHTTPTabData() {
	metrics := a.httpCollector.GetLatestMetrics()
	tab := a.tabs[1]

	// HTTP status table
	if len(tab.tables) > 0 {
		table := tab.tables[0]

		// Keep only the header row
		headerRow := table.Rows[0]
		table.Rows = make([][]string, 1)
		table.Rows[0] = headerRow

		// Store header style and reset row styles
		headerStyle := table.RowStyles[0]
		table.RowStyles = make(map[int]ui.Style)
		table.RowStyles[0] = headerStyle

		// Add data rows
		rowIdx := 1
		for name, metric := range metrics {
			status := "DOWN"
			statusColor := ui.ColorRed
			if metric.IsUp {
				status = "UP"
				statusColor = ui.ColorGreen
			}

			responseTime := fmt.Sprintf("%.2f ms", float64(metric.ResponseTime)/float64(time.Millisecond))

			// Use the GetEndpoints method to fetch endpoints from HTTP collector
			url := ""
			for _, ep := range a.httpCollector.GetEndpoints() {
				if ep.Name == name {
					url = ep.URL
					break
				}
			}

			table.Rows = append(table.Rows, []string{
				name,
				url,
				status,
				responseTime,
				metric.LastChecked.Format("15:04:05"),
			})

			// Set row style based on status
			table.RowStyles[rowIdx] = ui.NewStyle(statusColor)
			rowIdx++
		}
	}
}

// updateGitTabData updates the Git tab data
func (a *TermUIApp) updateGitTabData() {
	metrics := a.gitCollector.GetLatestMetrics()
	tab := a.tabs[2]

	// Git status table
	if len(tab.tables) > 0 {
		table := tab.tables[0]

		// Update table values
		if len(table.Rows) >= 7 {
			table.Rows[1][1] = metrics.Name
			table.Rows[2][1] = metrics.Branch
			table.Rows[3][1] = fmt.Sprintf("%d", metrics.CommitCount)

			lastCommitStr := "No commits yet"
			if !metrics.LastCommit.IsZero() {
				lastCommitStr = metrics.LastCommit.Format("2006-01-02 15:04:05")
			}
			table.Rows[4][1] = lastCommitStr

			modifiedColor := ui.StyleClear
			if metrics.ModifiedFiles > 0 {
				modifiedColor = ui.NewStyle(ui.ColorYellow)
			}
			table.Rows[5][1] = fmt.Sprintf("%d", metrics.ModifiedFiles)

			pendingColor := ui.StyleClear
			if metrics.PendingCommits > 0 {
				pendingColor = ui.NewStyle(ui.ColorYellow)
			}
			table.Rows[6][1] = fmt.Sprintf("%d", metrics.PendingCommits)

			// Update colors for modified files and pending commits
			if len(table.RowStyles) >= 7 {
				table.RowStyles[5] = modifiedColor
				table.RowStyles[6] = pendingColor
			}
		}
	}
}

// StartTermUIApp starts the terminal UI application
func StartTermUIApp(cfg *config.Config) error {
	app := NewTermUIApp(cfg)
	return app.Run()
}
