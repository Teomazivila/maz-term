package ui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SystemPanel displays system metrics
type SystemPanel struct {
	collector      *collector.SystemMetricsCollector
	metrics        models.SystemMetrics
	width          int
	height         int
	ctx            context.Context
	cancelFunc     context.CancelFunc
	refreshTicker  *time.Ticker
	subscription   chan models.SystemMetrics
	headerStyle    lipgloss.Style
	tableStyle     lipgloss.Style
	cpuTable       table.Model
	memoryTable    table.Model
	diskTable      table.Model
	networkTable   table.Model
	currentSection string
	sections       []string
}

// NewSystemPanel creates a new system metrics panel
func NewSystemPanel() *SystemPanel {
	ctx, cancel := context.WithCancel(context.Background())
	c := collector.NewSystemMetricsCollector()

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#25A065")).
		Padding(0, 1)

	tableStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	// Initialize CPU table
	cpuColumns := []table.Column{
		{Title: "Metric", Width: 20},
		{Title: "Value", Width: 20},
	}
	cpuRows := []table.Row{
		{"Avg CPU Usage", "0.00%"},
		{"Load Avg (1m)", "0.00"},
		{"Load Avg (5m)", "0.00"},
		{"Load Avg (15m)", "0.00"},
	}
	cpuTable := table.New(
		table.WithColumns(cpuColumns),
		table.WithRows(cpuRows),
		table.WithFocused(false),
		table.WithHeight(len(cpuRows)),
	)
	cpuTable.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
	})

	// Initialize Memory table
	memColumns := []table.Column{
		{Title: "Metric", Width: 20},
		{Title: "Value", Width: 20},
	}
	memRows := []table.Row{
		{"Memory Usage", "0.00%"},
		{"Total Memory", "0 MB"},
		{"Used Memory", "0 MB"},
		{"Free Memory", "0 MB"},
		{"Swap Usage", "0.00%"},
	}
	memTable := table.New(
		table.WithColumns(memColumns),
		table.WithRows(memRows),
		table.WithFocused(false),
		table.WithHeight(len(memRows)),
	)
	memTable.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
	})

	// Initialize Disk table
	diskColumns := []table.Column{
		{Title: "Mount", Width: 10},
		{Title: "Total", Width: 10},
		{Title: "Used", Width: 10},
		{Title: "Free", Width: 10},
		{Title: "Usage", Width: 10},
	}
	diskTable := table.New(
		table.WithColumns(diskColumns),
		table.WithRows([]table.Row{}),
		table.WithFocused(false),
		table.WithHeight(5),
	)
	diskTable.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
	})

	// Initialize Network table
	netColumns := []table.Column{
		{Title: "Interface", Width: 12},
		{Title: "Bytes Sent", Width: 12},
		{Title: "Bytes Recv", Width: 12},
		{Title: "Packets Sent", Width: 12},
		{Title: "Packets Recv", Width: 12},
	}
	netTable := table.New(
		table.WithColumns(netColumns),
		table.WithRows([]table.Row{}),
		table.WithFocused(false),
		table.WithHeight(5),
	)
	netTable.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
	})

	panel := &SystemPanel{
		collector:      c,
		ctx:            ctx,
		cancelFunc:     cancel,
		headerStyle:    headerStyle,
		tableStyle:     tableStyle,
		cpuTable:       cpuTable,
		memoryTable:    memTable,
		diskTable:      diskTable,
		networkTable:   netTable,
		currentSection: "CPU",
		sections:       []string{"CPU", "Memory", "Disk", "Network"},
	}

	// Start the collector
	go c.Start(ctx, 2*time.Second)

	// Subscribe to metrics updates
	panel.subscription = c.Subscribe()

	// Start a goroutine to process metrics updates
	go panel.processMetricsUpdates()

	return panel
}

// Init initializes the panel
func (p *SystemPanel) Init() tea.Cmd {
	// No initialization commands needed
	return nil
}

// Title returns the panel title
func (p *SystemPanel) Title() string {
	return "System Metrics"
}

// SetSize sets the panel size
func (p *SystemPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Update table heights based on available space
	availableHeight := height - 4 // Account for borders and header
	tableHeight := availableHeight
	if p.currentSection == "Disk" {
		if len(p.metrics.Disk.Filesystems) > 0 {
			tableHeight = min(tableHeight, len(p.metrics.Disk.Filesystems)+1) // +1 for header
		}
	} else if p.currentSection == "Network" {
		if len(p.metrics.Network.Interfaces) > 0 {
			tableHeight = min(tableHeight, len(p.metrics.Network.Interfaces)+1) // +1 for header
		}
	}

	p.cpuTable.SetHeight(tableHeight)
	p.memoryTable.SetHeight(tableHeight)
	p.diskTable.SetHeight(tableHeight)
	p.networkTable.SetHeight(tableHeight)

	// Update table widths
	tableWidth := width - 4 // Account for borders

	// Update column widths
	cpuColumns := p.cpuTable.Columns()
	memColumns := p.memoryTable.Columns()
	if len(cpuColumns) >= 2 && len(memColumns) >= 2 {
		metricWidth := tableWidth / 2
		valueWidth := tableWidth - metricWidth

		cpuColumns[0].Width = metricWidth
		cpuColumns[1].Width = valueWidth
		p.cpuTable.SetColumns(cpuColumns)

		memColumns[0].Width = metricWidth
		memColumns[1].Width = valueWidth
		p.memoryTable.SetColumns(memColumns)
	}

	// Update disk table columns
	if len(p.diskTable.Columns()) >= 5 {
		diskColumns := p.diskTable.Columns()
		colWidth := tableWidth / 5
		for i := range diskColumns {
			diskColumns[i].Width = colWidth
		}
		p.diskTable.SetColumns(diskColumns)
	}

	// Update network table columns
	if len(p.networkTable.Columns()) >= 5 {
		netColumns := p.networkTable.Columns()
		colWidth := tableWidth / 5
		for i := range netColumns {
			netColumns[i].Width = colWidth
		}
		p.networkTable.SetColumns(netColumns)
	}
}

// Update updates the panel
func (p *SystemPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			// Cycle through sections
			for i, section := range p.sections {
				if section == p.currentSection {
					p.currentSection = p.sections[(i+1)%len(p.sections)]
					break
				}
			}
		}
	}

	var cmd tea.Cmd
	switch p.currentSection {
	case "CPU":
		p.cpuTable, cmd = p.cpuTable.Update(msg)
	case "Memory":
		p.memoryTable, cmd = p.memoryTable.Update(msg)
	case "Disk":
		p.diskTable, cmd = p.diskTable.Update(msg)
	case "Network":
		p.networkTable, cmd = p.networkTable.Update(msg)
	}

	return p, cmd
}

// View renders the panel
func (p *SystemPanel) View() string {
	header := p.headerStyle.Render(fmt.Sprintf("%s (Tab to switch, current: %s)", p.Title(), p.currentSection))

	var content string
	switch p.currentSection {
	case "CPU":
		content = p.cpuTable.View()
	case "Memory":
		content = p.memoryTable.View()
	case "Disk":
		content = p.diskTable.View()
	case "Network":
		content = p.networkTable.View()
	}

	// Add table style
	content = p.tableStyle.Render(content)

	// Add last update time
	updateInfo := fmt.Sprintf("Last updated: %s", p.metrics.CollectedAt.Format("15:04:05"))
	updateStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#AAAAAA")).
		Align(lipgloss.Right)

	updateInfo = updateStyle.Render(updateInfo)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, updateInfo)
}

// Close properly cleans up resources used by the panel
func (p *SystemPanel) Close() {
	p.cancelFunc()
	if p.refreshTicker != nil {
		p.refreshTicker.Stop()
	}
	p.collector.Unsubscribe(p.subscription)
	p.collector.Stop()
}

// processMetricsUpdates processes metrics updates from the subscription channel
func (p *SystemPanel) processMetricsUpdates() {
	for metrics := range p.subscription {
		p.metrics = metrics
		p.updateTables()
	}
}

// updateTables updates the tables with the latest metrics
func (p *SystemPanel) updateTables() {
	// Update CPU table
	cpuRows := []table.Row{
		{"Avg CPU Usage", fmt.Sprintf("%.2f%%", p.metrics.CPU.UsagePercent)},
	}

	// Add Load Average
	cpuRows = append(cpuRows,
		table.Row{"Load Avg (1m)", fmt.Sprintf("%.2f", p.metrics.CPU.LoadAverage.Load1)},
		table.Row{"Load Avg (5m)", fmt.Sprintf("%.2f", p.metrics.CPU.LoadAverage.Load5)},
		table.Row{"Load Avg (15m)", fmt.Sprintf("%.2f", p.metrics.CPU.LoadAverage.Load15)},
	)

	// Add per-core usage if available
	for i, usage := range p.metrics.CPU.CoreUsage {
		cpuRows = append(cpuRows, table.Row{
			fmt.Sprintf("Core %d", i),
			fmt.Sprintf("%.2f%%", usage),
		})
	}

	p.cpuTable.SetRows(cpuRows)

	// Update Memory table
	memRows := []table.Row{
		{"Memory Usage", fmt.Sprintf("%.2f%%", p.metrics.Memory.UsagePercent)},
		{"Total Memory", formatBytes(p.metrics.Memory.Total)},
		{"Used Memory", formatBytes(p.metrics.Memory.Used)},
		{"Free Memory", formatBytes(p.metrics.Memory.Free)},
	}

	// Add swap info
	if p.metrics.Memory.SwapTotal > 0 {
		swapUsage := float64(p.metrics.Memory.SwapUsed) / float64(p.metrics.Memory.SwapTotal) * 100
		memRows = append(memRows,
			table.Row{"Swap Usage", fmt.Sprintf("%.2f%%", swapUsage)},
			table.Row{"Swap Total", formatBytes(p.metrics.Memory.SwapTotal)},
			table.Row{"Swap Used", formatBytes(p.metrics.Memory.SwapUsed)},
			table.Row{"Swap Free", formatBytes(p.metrics.Memory.SwapFree)},
		)
	}

	p.memoryTable.SetRows(memRows)

	// Update Disk table
	diskRows := []table.Row{}
	for _, fs := range p.metrics.Disk.Filesystems {
		diskRows = append(diskRows, table.Row{
			truncateString(fs.MountPoint, 10),
			formatBytes(fs.Total),
			formatBytes(fs.Used),
			formatBytes(fs.Free),
			fmt.Sprintf("%.1f%%", fs.UsagePercent),
		})
	}
	p.diskTable.SetRows(diskRows)

	// Update Network table
	netRows := []table.Row{}
	for _, iface := range p.metrics.Network.Interfaces {
		netRows = append(netRows, table.Row{
			truncateString(iface.Name, 12),
			formatBytes(iface.BytesSent),
			formatBytes(iface.BytesRecv),
			strconv.FormatUint(iface.PacketsSent, 10),
			strconv.FormatUint(iface.PacketsRecv, 10),
		})
	}
	p.networkTable.SetRows(netRows)
}

// formatBytes formats bytes as human-readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncateString truncates a string to the given max length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
