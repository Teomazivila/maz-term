package ui

import (
	"github.com/Teomazivila/maz-term/pkg/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DashboardTab represents a tab containing a dashboard layout
type DashboardTab struct {
	title  string
	layout *Layout
	width  int
	height int
	config config.LayoutTab
}

// NewDashboardTab creates a new dashboard tab
func NewDashboardTab(cfg config.LayoutTab) *DashboardTab {
	// Create a new dashboard tab
	tab := &DashboardTab{
		title:  cfg.Name,
		layout: NewLayout(1), // 1 pixel spacing between panels
		config: cfg,
	}

	// Initialize the layout rows
	for _, row := range cfg.Rows {
		// Create panels for this row
		panels := []Panel{}
		for _, panelName := range row.Panels {
			// Create a placeholder panel for now
			// This will be replaced with actual panels in Phase 3
			panels = append(panels, NewPlaceholderPanel(panelName))
		}

		// Add the row to the layout
		tab.layout.AddRow(row.Size, panels...)
	}

	return tab
}

// Title returns the tab title
func (d *DashboardTab) Title() string {
	return d.title
}

// Update handles updates for the tab
func (d *DashboardTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		// Handle key presses specific to this tab
		// Will be implemented in Phase 2
	}

	// Update the layout
	d.layout.Update()

	return d, nil
}

// View renders the tab
func (d *DashboardTab) View() string {
	// Render the layout
	content := d.layout.Render()

	// Style the content with the app base style
	return Theme.App.Render(content)
}

// SetSize sets the size of the tab
func (d *DashboardTab) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.layout.SetSize(width, height)
}

// PlaceholderPanel is a simple panel for testing layout
type PlaceholderPanel struct {
	title string
	data  string
}

// NewPlaceholderPanel creates a new placeholder panel
func NewPlaceholderPanel(title string) *PlaceholderPanel {
	return &PlaceholderPanel{
		title: title,
		data:  "Placeholder content for " + title,
	}
}

// Title returns the panel title
func (p *PlaceholderPanel) Title() string {
	return p.title
}

// Render renders the panel
func (p *PlaceholderPanel) Render(width, height int) string {
	// Create a style for the content
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	// Render the content
	return style.Render(p.data)
}

// Update updates the panel state
func (p *PlaceholderPanel) Update() {
	// No updates needed for a placeholder
}
