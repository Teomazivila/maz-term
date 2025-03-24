package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// HTTPPanel displays HTTP endpoint health status
type HTTPPanel struct {
	collector       *collector.HTTPHealthChecker
	metrics         map[string]models.EndpointMetrics
	width           int
	height          int
	ctx             context.Context
	cancelFunc      context.CancelFunc
	subscription    chan map[string]models.EndpointMetrics
	headerStyle     lipgloss.Style
	tableStyle      lipgloss.Style
	healthTable     table.Model
	endpoints       []models.EndpointConfig
	updateDebouncer *Debouncer
}

// NewHTTPPanel creates a new HTTP endpoint health panel
func NewHTTPPanel(endpoints []models.EndpointConfig) *HTTPPanel {
	ctx, cancel := context.WithCancel(context.Background())
	c := collector.NewHTTPHealthChecker(endpoints)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#4A7BF7")).
		Padding(0, 1)

	tableStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	// Initialize health table
	columns := []table.Column{
		{Title: "Endpoint", Width: 20},
		{Title: "URL", Width: 30},
		{Title: "Status", Width: 10},
		{Title: "Response", Width: 12},
		{Title: "Last Checked", Width: 20},
	}

	healthTable := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(false),
		table.WithHeight(10),
	)

	healthTable.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
	})

	panel := &HTTPPanel{
		collector:       c,
		ctx:             ctx,
		cancelFunc:      cancel,
		headerStyle:     headerStyle,
		tableStyle:      tableStyle,
		healthTable:     healthTable,
		endpoints:       endpoints,
		metrics:         make(map[string]models.EndpointMetrics),
		updateDebouncer: NewDebouncer(100 * time.Millisecond), // 100ms debounce for updates
	}

	// Start the collector
	go c.Start(ctx, 10*time.Second)

	// Subscribe to metrics updates
	panel.subscription = c.Subscribe()

	// Start a goroutine to process metrics updates
	go panel.processMetricsUpdates()

	return panel
}

// Init initializes the panel
func (p *HTTPPanel) Init() tea.Cmd {
	// No initialization commands needed
	return nil
}

// Title returns the panel title
func (p *HTTPPanel) Title() string {
	return "HTTP Endpoint Health"
}

// SetSize sets the panel size
func (p *HTTPPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Update table size
	tableHeight := MaxInt(height-4, 5) // Account for borders and header, minimum 5
	tableWidth := MaxInt(width-4, 40)  // Account for borders, minimum 40

	// Update table height
	p.healthTable.SetHeight(tableHeight)

	// Update column widths based on available space
	columns := p.healthTable.Columns()

	// Define proportion of total width for each column
	proportions := []float64{0.2, 0.35, 0.15, 0.15, 0.25}
	totalProportion := 0.0
	for _, prop := range proportions {
		totalProportion += prop
	}

	// Calculate actual widths, ensuring minimum widths
	minColumnWidths := []int{8, 15, 6, 8, 12} // Minimum widths for each column

	for i, prop := range proportions {
		if i < len(columns) {
			relativeWidth := int(float64(tableWidth) * (prop / totalProportion))
			// Ensure column is at least minimum width
			columns[i].Width = MaxInt(relativeWidth, minColumnWidths[i])
		}
	}

	p.healthTable.SetColumns(columns)
}

// MaxInt returns the maximum of two integers

// TruncateString truncates a string to the given length and adds "..." if truncated

// Update updates the panel
func (p *HTTPPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Handle window resize
		p.width = msg.Width
		p.height = msg.Height
		p.SetSize(msg.Width, msg.Height)
		return p, nil
	}

	// Update table
	p.healthTable, cmd = p.healthTable.Update(msg)
	return p, cmd
}

// View renders the panel
func (p *HTTPPanel) View() string {
	// Ensure we have minimum dimensions
	availWidth := MaxInt(p.width, 80)
	availHeight := MaxInt(p.height, 24)

	header := p.headerStyle.Render(p.Title())

	// Adjust table style to take up available space
	contentStyle := p.tableStyle.Copy().
		Width(availWidth - 4).  // Account for borders
		Height(availHeight - 6) // Account for header and update info

	content := contentStyle.Render(p.healthTable.View())

	// Add last update time
	updateTime := time.Now()
	for _, metric := range p.metrics {
		if metric.LastChecked.After(updateTime) {
			updateTime = metric.LastChecked
		}
	}

	updateInfo := fmt.Sprintf("Last checked: %s", updateTime.Format("15:04:05"))
	updateStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#AAAAAA")).
		Align(lipgloss.Right).
		Width(availWidth - 4)

	updateInfo = updateStyle.Render(updateInfo)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, updateInfo)
}

// Close properly cleans up resources used by the panel
func (p *HTTPPanel) Close() {
	p.cancelFunc()
	p.collector.Unsubscribe(p.subscription)
	p.collector.Stop()
}

// processMetricsUpdates processes metrics updates from the subscription channel
func (p *HTTPPanel) processMetricsUpdates() {
	for metrics := range p.subscription {
		// Store metrics immediately
		p.metrics = metrics

		// Debounce the UI update
		p.updateDebouncer.Debounce(func() {
			p.updateTable()
		})
	}
}

// updateTable updates the table with the latest metrics
func (p *HTTPPanel) updateTable() {
	rows := []table.Row{}

	// Sort endpoints by name to maintain consistent order
	for _, ep := range p.endpoints {
		if metric, ok := p.metrics[ep.Name]; ok {
			// Format status with color
			status := "DOWN"
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
			if metric.IsUp {
				status = "UP"
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
			}

			// Format response time
			responseTime := fmt.Sprintf("%.2f ms", float64(metric.ResponseTime)/float64(time.Millisecond))

			rows = append(rows, table.Row{
				ep.Name,
				TruncateString(ep.URL, 30),
				statusStyle.Render(status),
				responseTime,
				metric.LastChecked.Format("15:04:05"),
			})
		} else {
			// Endpoint not checked yet
			rows = append(rows, table.Row{
				ep.Name,
				TruncateString(ep.URL, 30),
				"PENDING",
				"--",
				"--",
			})
		}
	}

	p.healthTable.SetRows(rows)
}
