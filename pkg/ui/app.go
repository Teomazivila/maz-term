package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// StorageInterface represents a subset of the storage functionality needed by the UI
type StorageInterface interface {
	// System metrics history
	GetCPUUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetMemoryUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetDiskUsageHistory(mountPoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)

	// HTTP metrics history
	GetHTTPResponseTimeHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetHTTPAvailabilityHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetAllEndpoints() ([]string, error)

	// Git metrics history
	GetCommitCountHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetModifiedFilesHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetPendingCommitsHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)

	// Event annotations
	GetEventAnnotations(period time.Duration) ([]models.EventAnnotation, error)
	AddEventAnnotation(event models.EventAnnotation) error
	DeleteEventAnnotation(id string) error
}

// App represents the main terminal UI application using termui
type App struct {
	config            *config.Config
	activeTabIndex    int
	tabs              []*Tab
	statusBar         *widgets.Paragraph
	grid              *ui.Grid
	tabBar            *widgets.TabPane
	running           bool
	systemCollector   *collector.SystemMetricsCollector
	httpCollector     *collector.HTTPHealthChecker
	gitCollector      *collector.GitStatusCollector
	termWidth         int
	termHeight        int
	showHelp          bool
	helpPanel         *widgets.Paragraph
	storage           StorageInterface
	exportInProgress  bool
	historyRange      time.Duration            // Selected time range for history tab
	historyRangeIdx   int                      // Index of currently selected range option
	showAnnotations   bool                     // Whether to show event annotations
	annotations       []models.EventAnnotation // Cached event annotations
	addingAnnotation  bool                     // Whether we're currently adding a new annotation
	annotationForm    *widgets.Paragraph       // Form for adding new annotations
	zoomMode          bool                     // Whether we're in zoom mode
	zoomActiveChart   int                      // Index of chart being zoomed
	zoomStartPercent  float64                  // Start position of zoom region (percentage)
	zoomEndPercent    float64                  // End position of zoom region (percentage)
	zoomStartTime     time.Time                // Start time for zoomed view
	zoomEndTime       time.Time                // End time for zoomed view
	comparisonMode    bool                     // Whether comparison mode is active
	primaryMetric     string                   // Primary metric being compared
	comparisonMetrics []string                 // List of metrics being compared
}

// Tab represents a tab in the terminal UI
type Tab struct {
	name       string
	grid       *ui.Grid
	widgets    []ui.Drawable
	panels     []*widgets.Paragraph
	gauges     []*widgets.Gauge
	tables     []*widgets.Table
	sparklines []*widgets.SparklineGroup
	barCharts  []*widgets.BarChart
}

// NewApp creates a new terminal UI application
func NewApp(cfg *config.Config) *App {
	app := &App{
		config:          cfg,
		activeTabIndex:  0,
		tabs:            []*Tab{},
		historyRange:    24 * time.Hour, // Default to 24 hours
		historyRangeIdx: 3,              // Default index for 24 hours
		showAnnotations: true,           // Show annotations by default
	}

	// Create tabs from configuration
	for _, tab := range cfg.Layout {
		// Create a new dashboard tab
		app.tabs = append(app.tabs, NewTab(tab))
	}

	// Add History tab if not already included
	hasHistoryTab := false
	for _, tab := range app.tabs {
		if tab.name == "History" {
			hasHistoryTab = true
			break
		}
	}

	if !hasHistoryTab {
		// Add history tab
		app.tabs = append(app.tabs, NewTab(config.LayoutTab{
			Name:   "History",
			Panels: []string{"history"},
		}))
	}

	return app
}

// NewTab creates a new terminal UI tab
func NewTab(cfg config.LayoutTab) *Tab {
	return &Tab{
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
func (a *App) Run() error {
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

	// Render the initial UI
	a.updateData()
	a.updateLayout()

	a.running = true
	for a.running {
		select {
		case e := <-uiEvents:
			a.handleEvent(e)
		case <-ticker.C:
			a.updateData()
			a.updateLayout()
		}
	}

	return nil
}

// initCollectors initializes data collectors
func (a *App) initCollectors() {
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
func (a *App) createUI() {
	// Get terminal dimensions
	a.termWidth, a.termHeight = ui.TerminalDimensions()

	// Create tab bar with enhanced styling
	a.tabBar = widgets.NewTabPane(a.getTabNames()...)
	a.tabBar.Border = true
	a.tabBar.Title = "DevOps Dashboard"
	a.tabBar.BorderStyle.Fg = ui.ColorCyan
	a.tabBar.TitleStyle.Fg = ui.ColorCyan
	a.tabBar.TitleStyle.Modifier = ui.ModifierBold
	a.tabBar.ActiveTabStyle = ui.NewStyle(ui.ColorBlack, ui.ColorCyan, ui.ModifierBold)
	a.tabBar.InactiveTabStyle = ui.NewStyle(ui.ColorWhite, ui.ColorBlack)
	a.tabBar.SetRect(0, 0, a.termWidth, 3)

	// Create status bar with enhanced styling
	a.statusBar = widgets.NewParagraph()
	a.statusBar.Text = "Status: Ready"
	a.statusBar.Border = true
	a.statusBar.BorderStyle.Fg = ui.ColorCyan
	a.statusBar.TextStyle.Fg = ui.ColorWhite
	a.statusBar.SetRect(0, a.termHeight-3, a.termWidth, a.termHeight)

	// Create help panel with comprehensive information
	a.helpPanel = widgets.NewParagraph()
	a.helpPanel.Title = "Keyboard Shortcuts & Help"
	a.helpPanel.Border = true
	a.helpPanel.BorderStyle.Fg = ui.ColorCyan
	a.helpPanel.TitleStyle.Fg = ui.ColorCyan
	a.helpPanel.TitleStyle.Modifier = ui.ModifierBold
	a.helpPanel.Text = `
[Navigation]                [Actions]                     [History Tab]
[←/→] or [h/l] Switch Tabs  [r] Refresh Data             [[] Decrease Time Range
[1-4] Go to Tab Directly    [e] Export Data to CSV       []] Increase Time Range
[?] Toggle This Help        [q] Quit Application         [a] Toggle Annotations
[Esc] Close Help/Panels                                  [n] Add New Annotation
                                                        [z] Enter Zoom Mode
                                                        [c] Toggle Comparison Mode

[Zoom Mode]
[←/→] Move Zoom Window      [↑/↓] Resize Zoom Window     [Enter] Apply Zoom
[Esc] Exit Zoom Mode

[Comparison Mode]
[1-4] Select Primary Metric [Space] Toggle Comparison    [Esc] Exit Comparison Mode

[Tab Information]
System - CPU, memory, disk usage and system stats
HTTP   - Endpoint status and response time history 
Git    - Repository status and recent commit history
History - View historical metrics with adjustable time ranges and event annotations
`
	a.helpPanel.TextStyle.Fg = ui.ColorWhite
	// Position will be set in updateLayout

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
func (a *App) getTabNames() []string {
	names := make([]string, len(a.tabs))
	for i, tab := range a.tabs {
		names[i] = tab.name
	}
	return names
}

// createTabContent creates content for a specific tab
func (a *App) createTabContent(tab *Tab, tabIndex int) {
	// Create different content based on tab name
	switch tab.name {
	case "System", "System Overview":
		a.createSystemTabContent(tab)
	case "HTTP", "Services":
		a.createHTTPTabContent(tab)
	case "Git":
		a.createGitTabContent(tab)
	case "History":
		a.createHistoryTabContent(tab)
	}
}

// createSystemTabContent creates content for the System tab
func (a *App) createSystemTabContent(tab *Tab) {
	// Create CPU gauge with enhanced styling
	cpuGauge := widgets.NewGauge()
	cpuGauge.Title = "CPU Usage"
	cpuGauge.Percent = 0
	cpuGauge.BarColor = ui.ColorBlue
	cpuGauge.BorderStyle.Fg = ui.ColorCyan
	cpuGauge.TitleStyle.Fg = ui.ColorCyan
	cpuGauge.TitleStyle.Modifier = ui.ModifierBold
	cpuGauge.LabelStyle.Modifier = ui.ModifierBold
	tab.gauges = append(tab.gauges, cpuGauge)
	tab.widgets = append(tab.widgets, cpuGauge)

	// Create Memory gauge with enhanced styling
	memGauge := widgets.NewGauge()
	memGauge.Title = "Memory Usage"
	memGauge.Percent = 0
	memGauge.BarColor = ui.ColorGreen
	memGauge.BorderStyle.Fg = ui.ColorCyan
	memGauge.TitleStyle.Fg = ui.ColorCyan
	memGauge.TitleStyle.Modifier = ui.ModifierBold
	memGauge.LabelStyle.Modifier = ui.ModifierBold
	tab.gauges = append(tab.gauges, memGauge)
	tab.widgets = append(tab.widgets, memGauge)

	// Create CPU Sparklines with enhanced styling
	cpuSparkline := widgets.NewSparkline()
	cpuSparkline.Title = "CPU"
	cpuSparkline.LineColor = ui.ColorBlue
	cpuSparkline.TitleStyle.Fg = ui.ColorCyan
	cpuSparklines := widgets.NewSparklineGroup(cpuSparkline)
	cpuSparklines.Title = "CPU History"
	cpuSparklines.BorderStyle.Fg = ui.ColorCyan
	cpuSparklines.TitleStyle.Fg = ui.ColorCyan
	cpuSparklines.TitleStyle.Modifier = ui.ModifierBold
	tab.sparklines = append(tab.sparklines, cpuSparklines)
	tab.widgets = append(tab.widgets, cpuSparklines)

	// Create Disk Usage Bar Chart with enhanced styling
	diskChart := widgets.NewBarChart()
	diskChart.Title = "Disk Usage"
	diskChart.Labels = []string{"Root", "Home", "Data"}
	diskChart.Data = []float64{0, 0, 0}
	diskChart.BarWidth = 5
	diskChart.BarColors = []ui.Color{ui.ColorRed, ui.ColorGreen, ui.ColorBlue}
	diskChart.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}
	diskChart.NumStyles = []ui.Style{ui.NewStyle(ui.ColorBlack)}
	diskChart.BorderStyle.Fg = ui.ColorCyan
	diskChart.TitleStyle.Fg = ui.ColorCyan
	diskChart.TitleStyle.Modifier = ui.ModifierBold
	tab.barCharts = append(tab.barCharts, diskChart)
	tab.widgets = append(tab.widgets, diskChart)

	// Create Processes Table with enhanced styling
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
	processTable.BorderStyle.Fg = ui.ColorCyan
	processTable.TitleStyle.Fg = ui.ColorCyan
	processTable.TitleStyle.Modifier = ui.ModifierBold
	processTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
	tab.tables = append(tab.tables, processTable)
	tab.widgets = append(tab.widgets, processTable)
}

// createHTTPTabContent creates content for the HTTP tab
func (a *App) createHTTPTabContent(tab *Tab) {
	// Create HTTP status table with enhanced styling
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
	httpTable.BorderStyle.Fg = ui.ColorCyan
	httpTable.TitleStyle.Fg = ui.ColorCyan
	httpTable.TitleStyle.Modifier = ui.ModifierBold
	httpTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

	// Color status column
	httpTable.RowStyles[1] = ui.NewStyle(ui.ColorGreen)
	httpTable.RowStyles[2] = ui.NewStyle(ui.ColorGreen)
	httpTable.RowStyles[3] = ui.NewStyle(ui.ColorGreen)

	tab.tables = append(tab.tables, httpTable)
	tab.widgets = append(tab.widgets, httpTable)

	// Create response time plot with enhanced styling
	responsePlot := widgets.NewPlot()
	responsePlot.Title = "Response Time History"
	responsePlot.BorderStyle.Fg = ui.ColorCyan
	responsePlot.TitleStyle.Fg = ui.ColorCyan
	responsePlot.TitleStyle.Modifier = ui.ModifierBold
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
func (a *App) createGitTabContent(tab *Tab) {
	// Create Git status table with enhanced styling
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
	gitTable.BorderStyle.Fg = ui.ColorCyan
	gitTable.TitleStyle.Fg = ui.ColorCyan
	gitTable.TitleStyle.Modifier = ui.ModifierBold
	gitTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
	tab.tables = append(tab.tables, gitTable)
	tab.widgets = append(tab.widgets, gitTable)

	// Create commit history table with enhanced styling
	commitTable := widgets.NewTable()
	commitTable.Title = "Recent Commits"
	commitTable.Rows = [][]string{
		{"Hash", "Author", "Date", "Message"},
	}
	commitTable.TextStyle = ui.NewStyle(ui.ColorWhite)
	commitTable.RowSeparator = true
	commitTable.BorderStyle.Fg = ui.ColorCyan
	commitTable.TitleStyle.Fg = ui.ColorCyan
	commitTable.TitleStyle.Modifier = ui.ModifierBold
	commitTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
	tab.tables = append(tab.tables, commitTable)
	tab.widgets = append(tab.widgets, commitTable)
}

// createHistoryTabContent creates content for the History tab
func (a *App) createHistoryTabContent(tab *Tab) {
	// Create time range selector
	timeRangeSelector := widgets.NewParagraph()
	timeRangeSelector.Title = "Time Range"
	timeRangeSelector.Text = a.getTimeRangeDisplay()
	timeRangeSelector.BorderStyle.Fg = ui.ColorCyan
	timeRangeSelector.TitleStyle.Fg = ui.ColorCyan
	timeRangeSelector.TextStyle.Fg = ui.ColorWhite
	tab.panels = append(tab.panels, timeRangeSelector)
	tab.widgets = append(tab.widgets, timeRangeSelector)

	// Create annotation controls
	annotationControls := widgets.NewParagraph()
	annotationControls.Title = "Annotations"
	annotationControls.Text = "Press [a] to toggle annotations, [n] to add new annotation"
	annotationControls.BorderStyle.Fg = ui.ColorCyan
	annotationControls.TitleStyle.Fg = ui.ColorCyan
	annotationControls.TextStyle.Fg = ui.ColorWhite
	tab.panels = append(tab.panels, annotationControls)
	tab.widgets = append(tab.widgets, annotationControls)

	// Create CPU usage history plot
	cpuHistoryPlot := widgets.NewPlot()
	cpuHistoryPlot.Title = fmt.Sprintf("CPU Usage History (Last %s)", a.formatDuration(a.historyRange))
	cpuHistoryPlot.LineColors = []ui.Color{ui.ColorBlue}
	cpuHistoryPlot.AxesColor = ui.ColorWhite
	cpuHistoryPlot.BorderStyle.Fg = ui.ColorCyan
	cpuHistoryPlot.TitleStyle.Fg = ui.ColorCyan
	cpuHistoryPlot.TitleStyle.Modifier = ui.ModifierBold

	// Initialize with default data to prevent rendering errors
	cpuHistoryPlot.Data = [][]float64{{0, 0, 0}}
	cpuHistoryPlot.DrawDirection = widgets.DrawLeft
	tab.widgets = append(tab.widgets, cpuHistoryPlot)

	// Create memory usage history plot
	memHistoryPlot := widgets.NewPlot()
	memHistoryPlot.Title = fmt.Sprintf("Memory Usage History (Last %s)", a.formatDuration(a.historyRange))
	memHistoryPlot.LineColors = []ui.Color{ui.ColorGreen}
	memHistoryPlot.AxesColor = ui.ColorWhite
	memHistoryPlot.BorderStyle.Fg = ui.ColorCyan
	memHistoryPlot.TitleStyle.Fg = ui.ColorCyan
	memHistoryPlot.TitleStyle.Modifier = ui.ModifierBold

	// Initialize with default data to prevent rendering errors
	memHistoryPlot.Data = [][]float64{{0, 0, 0}}
	memHistoryPlot.DrawDirection = widgets.DrawLeft
	tab.widgets = append(tab.widgets, memHistoryPlot)

	// Create HTTP response time history plot
	httpHistoryPlot := widgets.NewPlot()
	httpHistoryPlot.Title = fmt.Sprintf("HTTP Response Time History (Last %s)", a.formatDuration(a.historyRange))
	httpHistoryPlot.LineColors = []ui.Color{ui.ColorRed, ui.ColorYellow, ui.ColorBlue}
	httpHistoryPlot.AxesColor = ui.ColorWhite
	httpHistoryPlot.BorderStyle.Fg = ui.ColorCyan
	httpHistoryPlot.TitleStyle.Fg = ui.ColorCyan
	httpHistoryPlot.TitleStyle.Modifier = ui.ModifierBold

	// Initialize with default data to prevent rendering errors - one series per color
	httpHistoryPlot.Data = [][]float64{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
	httpHistoryPlot.DrawDirection = widgets.DrawLeft
	tab.widgets = append(tab.widgets, httpHistoryPlot)

	// Create endpoints availability plot
	availabilityPlot := widgets.NewBarChart()
	availabilityPlot.Title = fmt.Sprintf("Endpoints Availability (Last %s)", a.formatDuration(a.historyRange))
	availabilityPlot.Labels = []string{"Endpoint1", "Endpoint2", "Endpoint3"}
	availabilityPlot.Data = []float64{100, 100, 100} // Default data
	availabilityPlot.BarColors = []ui.Color{ui.ColorGreen}
	availabilityPlot.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}
	availabilityPlot.NumStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}
	availabilityPlot.BorderStyle.Fg = ui.ColorCyan
	availabilityPlot.TitleStyle.Fg = ui.ColorCyan
	availabilityPlot.TitleStyle.Modifier = ui.ModifierBold
	tab.barCharts = append(tab.barCharts, availabilityPlot)
	tab.widgets = append(tab.widgets, availabilityPlot)

	// Create annotation form (hidden by default)
	annotationForm := widgets.NewParagraph()
	annotationForm.Title = "Add Annotation"
	annotationForm.Text = "Form will appear here when adding a new annotation"
	annotationForm.BorderStyle.Fg = ui.ColorCyan
	annotationForm.TitleStyle.Fg = ui.ColorCyan
	annotationForm.TextStyle.Fg = ui.ColorWhite
	a.annotationForm = annotationForm
	tab.panels = append(tab.panels, annotationForm)
	tab.widgets = append(tab.widgets, annotationForm)
}

// updateLayout updates the UI layout based on terminal dimensions
func (a *App) updateLayout() {
	// Get terminal dimensions
	width, height := ui.TerminalDimensions()
	if width != a.termWidth || height != a.termHeight {
		a.termWidth = width
		a.termHeight = height
	}

	// Calculate dimensions for components
	tabBarHeight := 3
	statusBarHeight := 3
	contentHeight := height - tabBarHeight - statusBarHeight

	// Set tab bar dimensions
	a.tabBar.SetRect(0, 0, width, tabBarHeight)

	// Set status bar dimensions
	a.statusBar.SetRect(0, height-statusBarHeight, width, height)

	// If help panel is shown, draw it centered
	if a.showHelp {
		helpWidth := 60
		helpHeight := 15
		helpX := (width - helpWidth) / 2
		helpY := (height - helpHeight) / 2

		a.helpPanel.SetRect(helpX, helpY, helpX+helpWidth, helpY+helpHeight)
		ui.Render(a.helpPanel)
		return
	}

	// Get active tab
	activeTab := a.tabs[a.tabBar.ActiveTabIndex]

	// Configure the tab content based on tab type
	switch activeTab.name {
	case "System", "System Overview":
		a.configureSystemTabGrid(activeTab, 0, tabBarHeight, width, tabBarHeight+contentHeight)
	case "HTTP", "Services":
		a.configureHTTPTabGrid(activeTab, 0, tabBarHeight, width, tabBarHeight+contentHeight)
	case "Git":
		a.configureGitTabGrid(activeTab, 0, tabBarHeight, width, tabBarHeight+contentHeight)
	case "History":
		a.configureHistoryTabGrid(activeTab, 0, tabBarHeight, width, tabBarHeight+contentHeight)
	}

	// Directly render each component to ensure they appear
	ui.Render(a.tabBar)

	// Render tab content
	if activeTab.grid != nil {
		ui.Render(activeTab.grid)
	} else if len(activeTab.widgets) > 0 {
		for _, widget := range activeTab.widgets {
			ui.Render(widget)
		}
	}

	// Render status bar with helpful key hints and current tab information
	activeTabName := "System"
	if a.tabBar.ActiveTabIndex >= 0 && a.tabBar.ActiveTabIndex < len(a.tabs) {
		activeTabName = a.tabs[a.tabBar.ActiveTabIndex].name
	}

	a.statusBar.Text = fmt.Sprintf("Status: Ready | Time: %s | Press [?] for Help | Tab %d/%d: %s",
		time.Now().Format("15:04:05"),
		a.tabBar.ActiveTabIndex+1,
		len(a.tabs),
		activeTabName)
	ui.Render(a.statusBar)
}

// configureSystemTabGrid configures the grid for the system tab
func (a *App) configureSystemTabGrid(tab *Tab, x1, y1, x2, y2 int) {
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
func (a *App) configureHTTPTabGrid(tab *Tab, x1, y1, x2, y2 int) {
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
func (a *App) configureGitTabGrid(tab *Tab, x1, y1, x2, y2 int) {
	if len(tab.widgets) < 2 {
		return
	}

	// Create a grid for the Git tab
	grid := ui.NewGrid()
	grid.SetRect(x1, y1, x2, y2)

	// Configure grid with status table and commit history table
	grid.Set(
		ui.NewRow(0.4, ui.NewCol(1.0, tab.widgets[0])), // Git status table
		ui.NewRow(0.6, ui.NewCol(1.0, tab.widgets[1])), // Commit history table
	)

	tab.grid = grid
}

// configureHistoryTabGrid configures the grid for the History tab
func (a *App) configureHistoryTabGrid(tab *Tab, x1, y1, x2, y2 int) {
	if len(tab.widgets) < 7 { // Includes control widgets and form
		return
	}

	// Create a grid for the history tab
	grid := ui.NewGrid()
	grid.SetRect(x1, y1, x2, y2)

	// Show annotation form if adding an annotation
	if a.addingAnnotation {
		// Configure grid with annotation form taking most of the space
		grid.Set(
			ui.NewRow(0.1, ui.NewCol(1.0, tab.widgets[0])), // Time range selector
			ui.NewRow(0.8, ui.NewCol(1.0, tab.widgets[6])), // Annotation form
			ui.NewRow(0.1, ui.NewCol(1.0, tab.widgets[1])), // Annotation controls
		)
	} else {
		// Configure grid with time range and annotation controls at the top, then plots in a 2x2 layout
		grid.Set(
			ui.NewRow(0.05, ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[0])), // Time range selector
				ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[1]))), // Annotation controls
			ui.NewRow(0.475,
				ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[2])), // CPU history plot
				ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[3])), // Memory history plot
			),
			ui.NewRow(0.475,
				ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[4])), // HTTP response time plot
				ui.NewCol(0.5, ui.NewRow(1.0, tab.widgets[5])), // Availability bar chart
			),
		)
	}

	tab.grid = grid
}

// handleEvent handles UI events
func (a *App) handleEvent(e ui.Event) {
	// If in zoom mode, handle zoom-specific controls
	if a.zoomMode {
		a.handleZoomModeEvent(e)
		return
	}

	// If in comparison mode, handle comparison-specific controls
	if a.comparisonMode {
		a.handleComparisonModeEvent(e)
		return
	}

	switch e.ID {
	case "q", "<C-c>":
		a.running = false
	case "<Resize>":
		payload := e.Payload.(ui.Resize)
		a.termWidth = payload.Width
		a.termHeight = payload.Height
		a.updateLayout()
	case "<Left>", "h":
		if !a.showHelp {
			a.tabBar.FocusLeft()
			a.updateLayout()
		}
	case "<Right>", "l":
		if !a.showHelp {
			a.tabBar.FocusRight()
			a.updateLayout()
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if !a.showHelp {
			idx := int(e.ID[0] - '1')
			if idx < len(a.tabs) {
				a.tabBar.ActiveTabIndex = idx
				a.updateLayout()
			}
		}
	case "?":
		a.showHelp = !a.showHelp
		a.updateLayout()
	case "<Escape>":
		if a.showHelp {
			a.showHelp = false
			a.updateLayout()
		} else if a.addingAnnotation {
			// Cancel adding annotation
			a.addingAnnotation = false
			a.updateLayout()
		}
	case "r":
		if !a.showHelp {
			// Check if we're in a zoomed view
			if a.tabBar.ActiveTabIndex == 3 && a.zoomEndTime.After(a.zoomStartTime) {
				// Reset zoom to default time range
				a.resetZoom()
			}

			// Force data refresh
			a.updateData()
			a.updateLayout()
		}
	case "e":
		if !a.showHelp && !a.exportInProgress && a.storage != nil {
			// Export data
			go a.exportData()
		}
	case "[", "<":
		// Decrease time range in history tab
		if !a.showHelp && a.tabBar.ActiveTabIndex == 3 {
			a.decreaseTimeRange()
			a.updateData()
			a.updateLayout()
		}
	case "]", ">":
		// Increase time range in history tab
		if !a.showHelp && a.tabBar.ActiveTabIndex == 3 {
			a.increaseTimeRange()
			a.updateData()
			a.updateLayout()
		}
	case "a":
		// Toggle annotations in history tab
		if !a.showHelp && a.tabBar.ActiveTabIndex == 3 {
			a.showAnnotations = !a.showAnnotations
			a.updateData()
			a.updateLayout()
		}
	case "n":
		// Add new annotation in history tab
		if !a.showHelp && a.tabBar.ActiveTabIndex == 3 && !a.addingAnnotation {
			a.addingAnnotation = true
			a.showAddAnnotationForm()
			a.updateLayout()
		}
	case "z":
		// Enter zoom mode for historical charts
		if !a.showHelp && a.tabBar.ActiveTabIndex == 3 && !a.addingAnnotation {
			a.enterZoomMode()
			a.updateLayout()
		}
	case "c":
		// Enter comparison mode in history tab
		if !a.showHelp && a.tabBar.ActiveTabIndex == 3 && !a.addingAnnotation && !a.zoomMode {
			a.enterComparisonMode()
			a.updateLayout()
		}
	}
}

// handleZoomModeEvent handles events when in zoom mode
func (a *App) handleZoomModeEvent(e ui.Event) {
	switch e.ID {
	case "<Escape>":
		// Exit zoom mode
		a.zoomMode = false
		a.updateLayout()
	case "<Left>":
		// Move zoom window left
		if a.zoomStartPercent > 0.05 {
			delta := 0.05
			a.zoomStartPercent -= delta
			a.zoomEndPercent -= delta
			a.updateZoomIndicator()
		}
	case "<Right>":
		// Move zoom window right
		if a.zoomEndPercent < 0.95 {
			delta := 0.05
			a.zoomStartPercent += delta
			a.zoomEndPercent += delta
			a.updateZoomIndicator()
		}
	case "<Up>":
		// Increase zoom window size
		if a.zoomEndPercent-a.zoomStartPercent < 0.9 {
			a.zoomStartPercent -= 0.05
			a.zoomEndPercent += 0.05

			// Ensure within bounds
			if a.zoomStartPercent < 0 {
				a.zoomStartPercent = 0
			}
			if a.zoomEndPercent > 1 {
				a.zoomEndPercent = 1
			}

			a.updateZoomIndicator()
		}
	case "<Down>":
		// Decrease zoom window size
		if a.zoomEndPercent-a.zoomStartPercent > 0.1 {
			a.zoomStartPercent += 0.05
			a.zoomEndPercent -= 0.05
			a.updateZoomIndicator()
		}
	case "<Enter>":
		// Apply zoom
		a.applyZoom()
		a.zoomMode = false
		a.updateData()
		a.updateLayout()
	}
}

// enterZoomMode enters zoom mode for a chart
func (a *App) enterZoomMode() {
	a.zoomMode = true

	// Default to first chart (CPU)
	a.zoomActiveChart = 2

	// Set initial zoom window to middle 50%
	a.zoomStartPercent = 0.25
	a.zoomEndPercent = 0.75

	// Calculate start and end times based on current time range
	now := time.Now()
	rangeStart := now.Add(-a.historyRange)

	totalDuration := now.Sub(rangeStart)
	a.zoomStartTime = rangeStart.Add(time.Duration(float64(totalDuration) * a.zoomStartPercent))
	a.zoomEndTime = rangeStart.Add(time.Duration(float64(totalDuration) * a.zoomEndPercent))

	// Update status bar to show zoom mode instructions
	a.statusBar.Text = "ZOOM MODE: Use [←/→] to move, [↑/↓] to resize, [Enter] to apply, [Esc] to cancel"
	ui.Render(a.statusBar)

	// Add zoom indicators to the active chart
	a.updateZoomIndicator()
}

// updateZoomIndicator updates the visual indicator for zoom mode
func (a *App) updateZoomIndicator() {
	if !a.zoomMode {
		return
	}

	// Get the active history tab
	tab := a.tabs[3]

	// Update all charts with zoom indicators
	for i := 2; i <= 4; i++ {
		if plot, ok := tab.widgets[i].(*widgets.Plot); ok {
			// Create a highlighted region representing the zoom window
			// For now, just update the title to show we're in zoom mode
			if i == a.zoomActiveChart {
				plot.Title = fmt.Sprintf("%s [ZOOM %.0f%%-%.0f%%](fg:yellow)",
					plot.Title, a.zoomStartPercent*100, a.zoomEndPercent*100)
			} else {
				originalTitle := plot.Title
				if idx := strings.Index(originalTitle, " [ZOOM"); idx > 0 {
					originalTitle = originalTitle[:idx]
				}
				plot.Title = originalTitle
			}
			ui.Render(plot)
		}
	}

	// Calculate zoom time range for status display
	now := time.Now()
	rangeStart := now.Add(-a.historyRange)
	totalDuration := now.Sub(rangeStart)

	zoomStartTime := rangeStart.Add(time.Duration(float64(totalDuration) * a.zoomStartPercent))
	zoomEndTime := rangeStart.Add(time.Duration(float64(totalDuration) * a.zoomEndPercent))

	a.zoomStartTime = zoomStartTime
	a.zoomEndTime = zoomEndTime

	// Format for display
	startTimeStr := zoomStartTime.Format("15:04:05")
	endTimeStr := zoomEndTime.Format("15:04:05")

	a.statusBar.Text = fmt.Sprintf("ZOOM MODE: %s to %s | Use [←/→] to move, [↑/↓] to resize, [Enter] to apply, [Esc] to cancel",
		startTimeStr, endTimeStr)
}

// applyZoom applies the current zoom selection
func (a *App) applyZoom() {
	// Create a custom time range based on zoom selection
	zoomDuration := a.zoomEndTime.Sub(a.zoomStartTime)

	// Set the new history range to the zoomed time range
	a.historyRange = zoomDuration

	// Update status to show we're using a custom time range
	a.statusBar.Text = fmt.Sprintf("Zoomed view: %s to %s | Press 'r' to reset zoom",
		a.zoomStartTime.Format("15:04:05"),
		a.zoomEndTime.Format("15:04:05"))
}

// updateData updates the data displayed in the UI
func (a *App) updateData() {
	// Don't update data while help is displayed
	if a.showHelp {
		return
	}

	// Update tab-specific data based on active tab
	activeTabIdx := a.tabBar.ActiveTabIndex
	switch activeTabIdx {
	case 0:
		a.updateSystemTabData()
	case 1:
		a.updateHTTPTabData()
	case 2:
		a.updateGitTabData()
	case 3:
		a.updateHistoryTabData()
	}
}

// updateSystemTabData updates the system tab data
func (a *App) updateSystemTabData() {
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

// updateHTTPTabData updates the HTTP tab data with improved visuals
func (a *App) updateHTTPTabData() {
	metrics := a.httpCollector.GetLatestMetrics()
	tab := a.tabs[1]

	// HTTP status table
	if len(tab.tables) > 0 {
		table := tab.tables[0]

		// Keep only the header row
		headerRow := table.Rows[0]
		table.Rows = [][]string{headerRow}

		// Save header style
		headerStyle := table.RowStyles[0]
		table.RowStyles = make(map[int]ui.Style)
		table.RowStyles[0] = headerStyle

		// Add data rows
		rowIdx := 1
		for name, metric := range metrics {
			// Determine status with better visual indicators
			status := "DOWN"
			statusColor := ui.ColorRed
			if metric.IsUp {
				status = "UP"
				statusColor = ui.ColorGreen
			}

			// Format the response time with color coding based on response time
			respTime := float64(metric.ResponseTime.Milliseconds())
			responseTime := fmt.Sprintf("%.0fms", respTime)

			// Format the last checked time
			lastChecked := metric.LastChecked.Format("15:04:05")

			// Use the endpoint URL from the collector configuration
			url := ""
			for _, ep := range a.httpCollector.GetEndpoints() {
				if ep.Name == name {
					url = ep.URL
					break
				}
			}

			// Add the row
			table.Rows = append(table.Rows, []string{
				name,
				url,
				status,
				responseTime,
				lastChecked,
			})

			// Set color based on status for the entire row
			table.RowStyles[rowIdx] = ui.NewStyle(statusColor)
			rowIdx++
		}

		// Add some placeholders if no data
		if len(metrics) == 0 {
			table.Rows = append(table.Rows, []string{
				"No endpoints", "", "N/A", "N/A", "N/A",
			})
			table.RowStyles[1] = ui.NewStyle(ui.ColorYellow)
		}
	}

	// Response time plot if we have any metrics
	if len(tab.widgets) > 1 && len(metrics) > 0 {
		// Try to display response time history
		plot, ok := tab.widgets[1].(*widgets.Plot)
		if ok {
			// Set plot title with stats
			plot.Title = fmt.Sprintf("Response Time History (Refresh: %ds)", 5)

			// Initialize data series for plot if needed
			if len(plot.Data) == 0 {
				plot.Data = make([][]float64, len(metrics))
				for i := range plot.Data {
					plot.Data[i] = make([]float64, 0, 20)
				}
			}

			// Update plot data with current metrics
			i := 0
			for _, metric := range metrics {
				// Add response time in milliseconds
				responseTime := float64(metric.ResponseTime.Milliseconds())

				// If we already have data for this endpoint
				if i < len(plot.Data) {
					// Limit data points to avoid excessive memory use
					if len(plot.Data[i]) >= 50 {
						plot.Data[i] = plot.Data[i][1:]
					}
					plot.Data[i] = append(plot.Data[i], responseTime)
				}
				i++
			}
		}
	}
}

// updateGitTabData updates the Git tab data with improved visuals
func (a *App) updateGitTabData() {
	metrics := a.gitCollector.GetLatestMetrics()
	tab := a.tabs[2]

	// Git status table
	if len(tab.tables) > 0 {
		table := tab.tables[0]

		// Make sure we have at least the minimum rows required
		if len(table.Rows) < 7 {
			table.Rows = [][]string{
				{"Property", "Value"},
				{"Repository", ""},
				{"Branch", ""},
				{"Commit Count", "0"},
				{"Last Commit", ""},
				{"Modified Files", "0"},
				{"Pending Commits", "0"},
			}
		}

		// Setup row styles if they don't exist
		if table.RowStyles == nil {
			table.RowStyles = make(map[int]ui.Style)
		}

		// Set header style
		table.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

		// Update table values with enhanced styling
		// Repository name
		table.Rows[1][1] = metrics.Name
		table.RowStyles[1] = ui.NewStyle(ui.ColorWhite)

		// Branch - highlight if not main/master
		table.Rows[2][1] = metrics.Branch
		if metrics.Branch != "main" && metrics.Branch != "master" {
			table.RowStyles[2] = ui.NewStyle(ui.ColorYellow)
		} else {
			table.RowStyles[2] = ui.NewStyle(ui.ColorWhite)
		}

		// Commit count
		table.Rows[3][1] = fmt.Sprintf("%d", metrics.CommitCount)
		table.RowStyles[3] = ui.NewStyle(ui.ColorWhite)

		// Format last commit time with a timestamp style
		lastCommitStr := "No commits yet"
		if !metrics.LastCommit.IsZero() {
			lastCommitStr = metrics.LastCommit.Format("2006-01-02 15:04:05")
			// Color code based on how recent the commit is
			elapsedHours := time.Since(metrics.LastCommit).Hours()
			if elapsedHours < 24 {
				table.RowStyles[4] = ui.NewStyle(ui.ColorGreen)
			} else if elapsedHours < 72 {
				table.RowStyles[4] = ui.NewStyle(ui.ColorYellow)
			} else {
				table.RowStyles[4] = ui.NewStyle(ui.ColorWhite)
			}
		} else {
			table.RowStyles[4] = ui.NewStyle(ui.ColorWhite)
		}
		table.Rows[4][1] = lastCommitStr

		// Update the number of modified files with proper coloring and prefixes
		if metrics.ModifiedFiles > 0 {
			table.Rows[5][1] = fmt.Sprintf("%d (uncommitted changes)", metrics.ModifiedFiles)
			table.RowStyles[5] = ui.NewStyle(ui.ColorYellow)
		} else {
			table.Rows[5][1] = "0 (clean working tree)"
			table.RowStyles[5] = ui.NewStyle(ui.ColorGreen)
		}

		// Update the number of pending commits with proper coloring and prefixes
		if metrics.PendingCommits > 0 {
			table.Rows[6][1] = fmt.Sprintf("%d (unpushed commits)", metrics.PendingCommits)
			table.RowStyles[6] = ui.NewStyle(ui.ColorYellow)
		} else {
			table.Rows[6][1] = "0 (synced with remote)"
			table.RowStyles[6] = ui.NewStyle(ui.ColorGreen)
		}
	}

	// Update commit history table
	if len(tab.tables) > 1 {
		commitTable := tab.tables[1]

		// Keep only header row
		headerRow := commitTable.Rows[0]
		commitTable.Rows = [][]string{headerRow}

		// Save header style
		if commitTable.RowStyles == nil {
			commitTable.RowStyles = make(map[int]ui.Style)
		}
		commitTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

		// Add commit history rows with enhanced styling
		if len(metrics.CommitHistory) > 0 {
			for i, commit := range metrics.CommitHistory {
				// Format hash to a shorter version with visual styling
				shortHash := commit.Hash
				if len(shortHash) > 8 {
					shortHash = shortHash[:8]
				}

				// Format commit time
				commitTime := "N/A"
				if !commit.Timestamp.IsZero() {
					commitTime = commit.Timestamp.Format("2006-01-02 15:04")
				}

				// Truncate message if too long
				message := commit.Message
				if len(message) > 50 {
					message = message[:47] + "..."
				}

				// Add row with commit info
				commitTable.Rows = append(commitTable.Rows, []string{
					shortHash,
					commit.Author,
					commitTime,
					message,
				})

				// Color code by commit age
				elapsedHours := time.Since(commit.Timestamp).Hours()
				if elapsedHours < 24 {
					// Recent commits (last 24 hours) in cyan
					commitTable.RowStyles[i+1] = ui.NewStyle(ui.ColorCyan)
				} else if elapsedHours < 168 { // 7 days
					// Commits from last week in white
					commitTable.RowStyles[i+1] = ui.NewStyle(ui.ColorWhite)
				} else {
					// Older commits in darker color
					commitTable.RowStyles[i+1] = ui.NewStyle(ui.ColorYellow)
				}
			}
		} else {
			// No commits available
			commitTable.Rows = append(commitTable.Rows, []string{
				"N/A", "N/A", "N/A", "No commits available",
			})
			commitTable.RowStyles[1] = ui.NewStyle(ui.ColorYellow)
		}
	}
}

// updateHistoryTabData updates the History tab data
func (a *App) updateHistoryTabData() {
	// Skip if storage is not available
	if a.storage == nil {
		return
	}

	tab := a.tabs[3]          // History tab
	if len(tab.widgets) < 6 { // Now includes controls widgets
		return
	}

	// Update time range selector
	timeRangeSelector, ok := tab.widgets[0].(*widgets.Paragraph)
	if ok {
		// If we're in a zoomed view, show custom zoom info
		if a.zoomMode {
			timeRangeSelector.Text = fmt.Sprintf("Time Range: ZOOM MODE (selecting %.0f%%-%.0f%%)",
				a.zoomStartPercent*100, a.zoomEndPercent*100)
		} else {
			timeRangeSelector.Text = a.getTimeRangeDisplay()
		}
	}

	// Update annotation controls
	annotationControls, ok := tab.widgets[1].(*widgets.Paragraph)
	if ok {
		if a.showAnnotations {
			annotationControls.Text = "Annotations: [ON](fg:green) Press [a] to toggle, [n] to add new, [z] to zoom"
		} else {
			annotationControls.Text = "Annotations: [OFF](fg:red) Press [a] to toggle, [n] to add new, [z] to zoom"
		}
	}

	// Define how much history to show based on selected range
	historyPeriod := a.historyRange
	pointCount := 100 // Number of data points to fetch

	// Fetch annotations if enabled
	if a.showAnnotations && a.storage != nil {
		annotations, err := a.storage.GetEventAnnotations(historyPeriod)
		if err == nil {
			a.annotations = annotations
		}
	}

	// Update CPU Usage History plot
	cpuPlot, ok := tab.widgets[2].(*widgets.Plot)
	if ok {
		cpuHistory, err := a.storage.GetCPUUsageHistory(historyPeriod, pointCount)
		if err == nil && len(cpuHistory) > 0 {
			// Convert time series data to plot data
			cpuData := make([]float64, len(cpuHistory))
			cpuTimeLabels := make([]string, len(cpuHistory))

			for i, point := range cpuHistory {
				cpuData[i] = point.Value
				cpuTimeLabels[i] = point.Timestamp.Format("15:04")
			}

			// Ensure there are at least 3 data points to avoid renderBraille errors
			if len(cpuData) < 3 {
				if len(cpuData) == 1 {
					// If one point, duplicate it twice
					cpuData = []float64{cpuData[0], cpuData[0], cpuData[0]}
				} else if len(cpuData) == 2 {
					// If two points, duplicate the second one
					cpuData = []float64{cpuData[0], cpuData[1], cpuData[1]}
				} else {
					// No points at all, use zeros
					cpuData = []float64{0, 0, 0}
				}
			}

			// Update the plot
			cpuPlot.Data = [][]float64{cpuData}

			// Set title with annotations if enabled
			baseTitle := fmt.Sprintf("CPU Usage History (Last %s)", a.formatDuration(historyPeriod))

			// If zoomed, show zoomed time range
			if !a.zoomMode && a.zoomEndTime.After(a.zoomStartTime) {
				zoomDuration := a.zoomEndTime.Sub(a.zoomStartTime)
				if zoomDuration > 0 {
					baseTitle = fmt.Sprintf("CPU Usage History (Zoomed: %s)",
						a.formatDuration(zoomDuration))
				}
			}

			if a.showAnnotations && len(a.annotations) > 0 {
				// Count relevant annotations
				cpuAnnotations := a.getAnnotationCount("cpu")
				if cpuAnnotations > 0 {
					baseTitle += fmt.Sprintf(" [%d events](fg:yellow)", cpuAnnotations)
				}
			}
			cpuPlot.Title = baseTitle
		} else {
			// Set default data with 3 points to avoid plot rendering issues
			cpuPlot.Data = [][]float64{{0, 0, 0}}
			cpuPlot.Title = fmt.Sprintf("CPU Usage History (Last %s)", a.formatDuration(historyPeriod))
		}
	}

	// Update Memory Usage History plot
	memPlot, ok := tab.widgets[3].(*widgets.Plot)
	if ok {
		memHistory, err := a.storage.GetMemoryUsageHistory(historyPeriod, pointCount)
		if err == nil && len(memHistory) > 0 {
			// Convert time series data to plot data
			memData := make([]float64, len(memHistory))
			memTimeLabels := make([]string, len(memHistory))

			for i, point := range memHistory {
				memData[i] = point.Value
				memTimeLabels[i] = point.Timestamp.Format("15:04")
			}

			// Ensure there are at least 3 data points to avoid renderBraille errors
			if len(memData) < 3 {
				if len(memData) == 1 {
					// If one point, duplicate it twice
					memData = []float64{memData[0], memData[0], memData[0]}
				} else if len(memData) == 2 {
					// If two points, duplicate the second one
					memData = []float64{memData[0], memData[1], memData[1]}
				} else {
					// No points at all, use zeros
					memData = []float64{0, 0, 0}
				}
			}

			// Update the plot
			memPlot.Data = [][]float64{memData}

			// Set title with annotations if enabled
			baseTitle := fmt.Sprintf("Memory Usage History (Last %s)", a.formatDuration(historyPeriod))

			// If zoomed, show zoomed time range
			if !a.zoomMode && a.zoomEndTime.After(a.zoomStartTime) {
				zoomDuration := a.zoomEndTime.Sub(a.zoomStartTime)
				if zoomDuration > 0 {
					baseTitle = fmt.Sprintf("Memory Usage History (Zoomed: %s)",
						a.formatDuration(zoomDuration))
				}
			}

			if a.showAnnotations && len(a.annotations) > 0 {
				// Count relevant annotations
				memAnnotations := a.getAnnotationCount("memory")
				if memAnnotations > 0 {
					baseTitle += fmt.Sprintf(" [%d events](fg:yellow)", memAnnotations)
				}
			}
			memPlot.Title = baseTitle
		} else {
			// Set default data with 3 points to avoid plot rendering issues
			memPlot.Data = [][]float64{{0, 0, 0}}
			memPlot.Title = fmt.Sprintf("Memory Usage History (Last %s)", a.formatDuration(historyPeriod))
		}
	}

	// Update HTTP Response Time History plot
	httpPlot, ok := tab.widgets[4].(*widgets.Plot)
	if ok {
		// Get all endpoints
		endpoints, err := a.storage.GetAllEndpoints()
		seriesCount := 3 // Default to 3 data series

		if err == nil && len(endpoints) > 0 {
			// Cap the number of endpoints to show (max 3)
			seriesCount = len(endpoints)
			if seriesCount > 3 {
				seriesCount = 3
			}

			// Create empty data arrays of correct size for each endpoint
			httpData := make([][]float64, seriesCount)
			for i := range httpData {
				httpData[i] = []float64{0, 0, 0} // Default with at least 3 points
			}

			// Collect response time history for each endpoint
			for i := 0; i < seriesCount; i++ {
				history, err := a.storage.GetHTTPResponseTimeHistory(endpoints[i], historyPeriod, pointCount)
				if err == nil && len(history) > 0 {
					// Convert time series data to plot data
					data := make([]float64, len(history))
					for j, point := range history {
						data[j] = point.Value
					}

					// Ensure there are at least 3 data points to avoid rendering errors
					if len(data) < 3 {
						if len(data) == 1 {
							// If one point, duplicate it twice
							data = []float64{data[0], data[0], data[0]}
						} else if len(data) == 2 {
							// If two points, duplicate the second one
							data = []float64{data[0], data[1], data[1]}
						} else {
							// No points at all, use zeros
							data = []float64{0, 0, 0}
						}
					}

					httpData[i] = data
				}
				// If error or no data, the default 3-point zero data remains
			}

			// Update the plot
			httpPlot.Data = httpData

			// Set title with annotations if enabled
			baseTitle := fmt.Sprintf("HTTP Response Time History (Last %s)", a.formatDuration(historyPeriod))

			// If zoomed, show zoomed time range
			if !a.zoomMode && a.zoomEndTime.After(a.zoomStartTime) {
				zoomDuration := a.zoomEndTime.Sub(a.zoomStartTime)
				if zoomDuration > 0 {
					baseTitle = fmt.Sprintf("HTTP Response Time History (Zoomed: %s)",
						a.formatDuration(zoomDuration))
				}
			}

			if a.showAnnotations && len(a.annotations) > 0 {
				// Count relevant annotations
				httpAnnotations := a.getAnnotationCount("http")
				if httpAnnotations > 0 {
					baseTitle += fmt.Sprintf(" [%d events](fg:yellow)", httpAnnotations)
				}
			}
			httpPlot.Title = baseTitle
		} else {
			// Set default data - 3 series with 3 points each
			httpPlot.Data = [][]float64{
				{0, 0, 0},
				{0, 0, 0},
				{0, 0, 0},
			}
			httpPlot.Title = fmt.Sprintf("HTTP Response Time History (Last %s)", a.formatDuration(historyPeriod))
		}
	}

	// Update Endpoints Availability bar chart
	availabilityChart, ok := tab.widgets[5].(*widgets.BarChart)
	if ok {
		// Get all endpoints
		endpoints, err := a.storage.GetAllEndpoints()
		if err == nil && len(endpoints) > 0 {
			// Cap the number of endpoints to show
			endpointCount := len(endpoints)
			if endpointCount > 6 {
				endpointCount = 6
			}

			// Create data arrays
			availData := make([]float64, endpointCount)
			availLabels := make([]string, endpointCount)

			// Collect availability history for each endpoint
			for i := 0; i < endpointCount; i++ {
				history, err := a.storage.GetHTTPAvailabilityHistory(endpoints[i], historyPeriod, 1)
				if err == nil && len(history) > 0 {
					// Use the latest availability point
					availData[i] = history[len(history)-1].Value
				} else {
					availData[i] = 0 // No data
				}
				availLabels[i] = endpoints[i]
			}

			// If no endpoints, provide default data
			if endpointCount == 0 {
				availData = []float64{0}
				availLabels = []string{"No Endpoints"}
			}

			// Update the bar chart
			availabilityChart.Data = availData
			availabilityChart.Labels = availLabels

			// Set title with annotations if enabled
			baseTitle := fmt.Sprintf("Endpoints Availability (Last %s)", a.formatDuration(historyPeriod))

			// If zoomed, show zoomed time range
			if !a.zoomMode && a.zoomEndTime.After(a.zoomStartTime) {
				zoomDuration := a.zoomEndTime.Sub(a.zoomStartTime)
				if zoomDuration > 0 {
					baseTitle = fmt.Sprintf("Endpoints Availability (Zoomed: %s)",
						a.formatDuration(zoomDuration))
				}
			}

			if a.showAnnotations && len(a.annotations) > 0 {
				// Count relevant annotations
				availAnnotations := a.getAnnotationCount("availability")
				if availAnnotations > 0 {
					baseTitle += fmt.Sprintf(" [%d events](fg:yellow)", availAnnotations)
				}
			}
			availabilityChart.Title = baseTitle
		} else {
			// Set default data to avoid rendering issues
			availabilityChart.Data = []float64{0}
			availabilityChart.Labels = []string{"No Endpoints"}
			availabilityChart.Title = fmt.Sprintf("Endpoints Availability (Last %s)", a.formatDuration(historyPeriod))
		}
	}
}

// StartApp starts the terminal UI application
func StartApp(cfg *config.Config, storageProvider collector.StorageProvider) error {
	app := NewApp(cfg)

	// Set the storage provider if available
	if storageProvider != nil {
		app.SetStorageProvider(storageProvider)
	}

	return app.Run()
}

// SetStorageProvider sets the storage provider for all collectors
func (a *App) SetStorageProvider(provider collector.StorageProvider) {
	if a.systemCollector != nil {
		a.systemCollector.SetStorageProvider(provider)
	}

	if a.httpCollector != nil {
		a.httpCollector.SetStorageProvider(provider)
	}

	if a.gitCollector != nil {
		a.gitCollector.SetStorageProvider(provider)
	}

	// Store the StorageInterface for the UI if it implements it
	if storageInterface, ok := provider.(StorageInterface); ok {
		a.storage = storageInterface
	}
}

// exportData exports metrics data to CSV files
func (a *App) exportData() {
	// Check if storage is available
	if a.storage == nil {
		return
	}

	// Set export in progress flag
	a.exportInProgress = true

	// Update status bar to show export is in progress
	a.statusBar.Text = "Status: Exporting data to CSV files... Please wait."
	ui.Render(a.statusBar)

	// Try to get the storage adapter through type assertion
	exporter, ok := a.storage.(interface {
		ExportData(options interface{}) error
	})

	if !ok {
		// If we can't get the exporter interface, update status and return
		a.statusBar.Text = "Status: Export failed - storage provider doesn't support export"
		ui.Render(a.statusBar)
		a.exportInProgress = false
		return
	}

	// Create export directory in user's Downloads folder
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	outputDir := filepath.Join(homeDir, "Downloads", "maz-term_export")

	// Create options for export
	options := map[string]interface{}{
		"OutputDir":     outputDir,
		"Period":        7 * 24 * time.Hour, // Last 7 days
		"IncludeSystem": true,
		"IncludeHTTP":   true,
		"IncludeGit":    true,
	}

	// Perform the export
	err = exporter.ExportData(options)

	// Reset export in progress flag
	a.exportInProgress = false

	// Update status with result
	if err != nil {
		a.statusBar.Text = fmt.Sprintf("Status: Export failed - %s", err.Error())
	} else {
		a.statusBar.Text = fmt.Sprintf("Status: Data exported successfully to %s", outputDir)
	}
	ui.Render(a.statusBar)

	// Reset status bar after 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		if !a.exportInProgress {
			a.statusBar.Text = fmt.Sprintf("Status: Ready | Time: %s | Press [?] for Help | Tab %d/%d: %s",
				time.Now().Format("15:04:05"),
				a.tabBar.ActiveTabIndex+1,
				len(a.tabs),
				a.tabs[a.tabBar.ActiveTabIndex].name)
			ui.Render(a.statusBar)
		}
	}()
}

// Helper methods for time range selection
func (a *App) getTimeRangeOptions() []time.Duration {
	return []time.Duration{
		1 * time.Hour,      // 1 hour
		6 * time.Hour,      // 6 hours
		12 * time.Hour,     // 12 hours
		24 * time.Hour,     // 24 hours
		3 * 24 * time.Hour, // 3 days
		7 * 24 * time.Hour, // 7 days
	}
}

func (a *App) getTimeRangeDisplay() string {
	labels := []string{"1h", "6h", "12h", "24h", "3d", "7d"}

	displayStr := "Time Range: "
	for i, label := range labels {
		if i == a.historyRangeIdx {
			displayStr += fmt.Sprintf("[%s](fg:black,bg:cyan) ", label)
		} else {
			displayStr += fmt.Sprintf("[%s](fg:white) ", label)
		}
	}
	displayStr += "\nUse [[] and []] keys to change"

	return displayStr
}

func (a *App) formatDuration(d time.Duration) string {
	hours := d.Hours()
	if hours < 24 {
		return fmt.Sprintf("%.0f Hours", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.0f Days", days)
}

func (a *App) decreaseTimeRange() {
	options := a.getTimeRangeOptions()
	if a.historyRangeIdx > 0 {
		a.historyRangeIdx--
		a.historyRange = options[a.historyRangeIdx]
	}
}

func (a *App) increaseTimeRange() {
	options := a.getTimeRangeOptions()
	if a.historyRangeIdx < len(options)-1 {
		a.historyRangeIdx++
		a.historyRange = options[a.historyRangeIdx]
	}
}

// showAddAnnotationForm displays the form for adding a new annotation
func (a *App) showAddAnnotationForm() {
	if a.annotationForm == nil {
		return
	}

	// Set form content
	now := time.Now()
	formContent := fmt.Sprintf(`
[Title](fg:white,mod:bold): [New Event](fg:cyan)
[Time](fg:white,mod:bold): [%s](fg:cyan)
[Type](fg:white,mod:bold): [deployment](fg:cyan) | [restart](fg:white) | [alert](fg:white) | [incident](fg:white) | [maintenance](fg:white) | [other](fg:white)
[Severity](fg:white,mod:bold): [info](fg:cyan) | [warning](fg:white) | [critical](fg:white)
[Description](fg:white,mod:bold): [Enter description here](fg:cyan)
[Tags](fg:white,mod:bold): [cpu,system](fg:cyan)

This is a placeholder for a form. In a full implementation, you would be able to navigate fields,
edit values, and save the annotation to the database.

Press [Enter] to submit or [Esc] to cancel.
`, now.Format("2006-01-02 15:04:05"))

	a.annotationForm.Text = formContent
}

// getAnnotationCount returns the count of annotations with a given tag
func (a *App) getAnnotationCount(tag string) int {
	count := 0
	for _, ann := range a.annotations {
		for _, t := range ann.Tags {
			if t == tag {
				count++
				break
			}
		}
	}
	return count
}

// resetZoom resets from a zoomed view back to the original time range
func (a *App) resetZoom() {
	// Reset the time range to the corresponding predefined option
	a.historyRange = a.getTimeRangeOptions()[a.historyRangeIdx]

	// Clear zoom times
	a.zoomStartTime = time.Time{}
	a.zoomEndTime = time.Time{}

	// Update status bar
	a.statusBar.Text = fmt.Sprintf("Zoom reset to %s | Time: %s",
		a.formatDuration(a.historyRange),
		time.Now().Format("15:04:05"))
	ui.Render(a.statusBar)
}

// handleComparisonModeEvent handles events when in comparison mode
func (a *App) handleComparisonModeEvent(e ui.Event) {
	switch e.ID {
	case "<Escape>":
		// Exit comparison mode
		a.exitComparisonMode()
		a.updateLayout()
	case "1":
		// Select CPU as primary metric
		a.primaryMetric = "cpu"
		a.updateComparisonView()
	case "2":
		// Select Memory as primary metric
		a.primaryMetric = "memory"
		a.updateComparisonView()
	case "3":
		// Select HTTP as primary metric
		a.primaryMetric = "http"
		a.updateComparisonView()
	case "4":
		// Select Availability as primary metric
		a.primaryMetric = "availability"
		a.updateComparisonView()
	case "<Space>":
		// Toggle individual metrics for comparison
		a.toggleComparisonMetric()
		a.updateComparisonView()
	}
}

// enterComparisonMode enters comparison mode for metrics
func (a *App) enterComparisonMode() {
	a.comparisonMode = true

	// Default to CPU as primary metric
	a.primaryMetric = "cpu"

	// Default comparisons
	a.comparisonMetrics = []string{"memory"}

	// Update status bar
	a.statusBar.Text = "COMPARISON MODE: Press [1-4] to select primary metric, [Space] to toggle comparisons, [Esc] to exit"
	ui.Render(a.statusBar)

	// Update the view
	a.updateComparisonView()
}

// exitComparisonMode exits comparison mode
func (a *App) exitComparisonMode() {
	a.comparisonMode = false
	a.primaryMetric = ""
	a.comparisonMetrics = nil

	// Reset status bar
	a.statusBar.Text = fmt.Sprintf("Status: Ready | Time: %s | Press [?] for Help | Tab %d/%d: %s",
		time.Now().Format("15:04:05"),
		a.tabBar.ActiveTabIndex+1,
		len(a.tabs),
		a.tabs[a.tabBar.ActiveTabIndex].name)
}

// toggleComparisonMetric toggles a metric for comparison
func (a *App) toggleComparisonMetric() {
	// List of all possible metrics
	allMetrics := []string{"cpu", "memory", "http", "availability"}

	// For now, we just rotate through possible comparison combinations
	// In a real implementation, we would have a UI to select specific metrics

	// Remove primary metric from possible comparisons
	possibleComparisons := []string{}
	for _, m := range allMetrics {
		if m != a.primaryMetric {
			possibleComparisons = append(possibleComparisons, m)
		}
	}

	// Simple rotation through comparison combinations
	switch len(a.comparisonMetrics) {
	case 0:
		// Add first comparison
		a.comparisonMetrics = []string{possibleComparisons[0]}
	case 1:
		// Add second comparison
		a.comparisonMetrics = []string{possibleComparisons[0], possibleComparisons[1]}
	case 2:
		// Add third comparison
		a.comparisonMetrics = []string{possibleComparisons[0], possibleComparisons[1], possibleComparisons[2]}
	default:
		// Start over with no comparisons
		a.comparisonMetrics = []string{}
	}
}

// updateComparisonView updates the comparison view
func (a *App) updateComparisonView() {
	if !a.comparisonMode {
		return
	}

	// Get the active history tab
	tab := a.tabs[3]

	// Update the status bar with current comparison info
	comparisonInfo := fmt.Sprintf("COMPARISON MODE: Primary: [%s](fg:cyan) | Comparing with: ", a.primaryMetric)

	if len(a.comparisonMetrics) == 0 {
		comparisonInfo += "[none](fg:yellow)"
	} else {
		for i, metric := range a.comparisonMetrics {
			if i > 0 {
				comparisonInfo += ", "
			}
			comparisonInfo += fmt.Sprintf("[%s](fg:green)", metric)
		}
	}

	a.statusBar.Text = comparisonInfo
	ui.Render(a.statusBar)

	// Update the time range selector to show comparison mode
	timeRangeSelector, ok := tab.widgets[0].(*widgets.Paragraph)
	if ok {
		timeRangeSelector.Text = fmt.Sprintf("Time Range: %s | COMPARISON MODE: Primary metric: %s",
			a.formatDuration(a.historyRange), a.primaryMetric)
		ui.Render(timeRangeSelector)
	}

	// Create a combined plot for the comparison
	combinedPlot, ok := tab.widgets[2].(*widgets.Plot)
	if ok {
		// Update the plot title
		combinedPlot.Title = fmt.Sprintf("Metric Comparison - Primary: %s (Last %s)",
			strings.ToUpper(a.primaryMetric), a.formatDuration(a.historyRange))

		// Generate data for primary metric
		primaryData := a.getMetricData(a.primaryMetric)

		// Create the data array starting with primary
		plotData := [][]float64{primaryData}

		// Add comparison metrics
		for _, metric := range a.comparisonMetrics {
			plotData = append(plotData, a.getMetricData(metric))
		}

		// Set line colors based on metrics
		combinedPlot.LineColors = a.getMetricColors(a.primaryMetric, a.comparisonMetrics)

		// Update the plot data
		combinedPlot.Data = plotData
		ui.Render(combinedPlot)
	}

	// Hide other plots in comparison mode
	for i := 3; i <= 5; i++ {
		widget := tab.widgets[i]

		// Make the widget invisible in comparison mode
		// For now, we'll just change the title to indicate it's hidden
		if plot, ok := widget.(*widgets.Plot); ok {
			plot.Title = "[HIDDEN IN COMPARISON MODE](fg:yellow)"
			ui.Render(plot)
		} else if barChart, ok := widget.(*widgets.BarChart); ok {
			barChart.Title = "[HIDDEN IN COMPARISON MODE](fg:yellow)"
			ui.Render(barChart)
		}
	}
}

// getMetricData gets the data for a specific metric
func (a *App) getMetricData(metricName string) []float64 {
	// Default return value with empty data
	defaultData := []float64{0, 0, 0}

	// Skip if storage is not available
	if a.storage == nil {
		return defaultData
	}

	historyPeriod := a.historyRange
	pointCount := 100

	var data []float64
	var err error
	var history []models.TimeSeriesPoint

	switch metricName {
	case "cpu":
		history, err = a.storage.GetCPUUsageHistory(historyPeriod, pointCount)
	case "memory":
		history, err = a.storage.GetMemoryUsageHistory(historyPeriod, pointCount)
	case "http":
		// For HTTP, we'll use the first endpoint as an example
		endpoints, err := a.storage.GetAllEndpoints()
		if err != nil || len(endpoints) == 0 {
			return defaultData
		}
		history, err = a.storage.GetHTTPResponseTimeHistory(endpoints[0], historyPeriod, pointCount)
	case "availability":
		// For availability, we'll use the first endpoint as an example
		endpoints, err := a.storage.GetAllEndpoints()
		if err != nil || len(endpoints) == 0 {
			return defaultData
		}
		history, err = a.storage.GetHTTPAvailabilityHistory(endpoints[0], historyPeriod, pointCount)
	default:
		return defaultData
	}

	if err != nil || len(history) == 0 {
		return defaultData
	}

	// Convert time series data to plot data
	data = make([]float64, len(history))
	for i, point := range history {
		data[i] = point.Value
	}

	// Ensure we have at least 3 data points (for rendering)
	if len(data) < 3 {
		if len(data) == 1 {
			data = []float64{data[0], data[0], data[0]}
		} else if len(data) == 2 {
			data = []float64{data[0], data[1], data[1]}
		} else {
			data = defaultData
		}
	}

	return data
}

// getMetricColors returns colors for metrics
func (a *App) getMetricColors(primary string, comparisons []string) []ui.Color {
	// Base set of colors
	colors := []ui.Color{}

	// Primary metric is always the first and gets a specific color
	switch primary {
	case "cpu":
		colors = append(colors, ui.ColorBlue)
	case "memory":
		colors = append(colors, ui.ColorGreen)
	case "http":
		colors = append(colors, ui.ColorRed)
	case "availability":
		colors = append(colors, ui.ColorYellow)
	default:
		colors = append(colors, ui.ColorCyan)
	}

	// Additional colors for comparison metrics
	for _, metric := range comparisons {
		switch metric {
		case "cpu":
			colors = append(colors, ui.ColorBlue)
		case "memory":
			colors = append(colors, ui.ColorGreen)
		case "http":
			colors = append(colors, ui.ColorRed)
		case "availability":
			colors = append(colors, ui.ColorYellow)
		default:
			colors = append(colors, ui.ColorWhite)
		}
	}

	return colors
}
