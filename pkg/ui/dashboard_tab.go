package ui

import (
	"context"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DashboardTab represents a tab containing a dashboard layout
type DashboardTab struct {
	title      string
	layout     *Layout
	width      int
	height     int
	config     config.LayoutTab
	components []tea.Model
	context    context.Context
}

// NewDashboardTab creates a new dashboard tab
func NewDashboardTab(cfg config.LayoutTab) *DashboardTab {
	// Create a new dashboard tab
	tab := &DashboardTab{
		title:  cfg.Name,
		layout: NewLayout(1), // 1 pixel spacing between panels
		config: cfg,
	}

	// Initialize with a single row for all panels
	panels := []Panel{}
	for _, panelName := range cfg.Panels {
		panel := tab.createPanel(panelName)
		panels = append(panels, panel)
	}

	// Add all panels to a single row
	tab.layout.AddRow(1, panels...)

	return tab
}

// createPanel creates a specific panel based on its name
func (d *DashboardTab) createPanel(name string) Panel {
	switch name {
	case "system":
		return NewSystemPanel()
	case "http":
		// Try to get endpoints from the application config
		var endpoints []models.EndpointConfig

		// We need to get the global config from somewhere
		// For now, just use default endpoints
		endpoints = []models.EndpointConfig{
			{Name: "Google", URL: "https://www.google.com", Method: "GET"},
			{Name: "GitHub", URL: "https://github.com", Method: "GET"},
			{Name: "Example", URL: "https://example.com", Method: "GET"},
		}

		return NewHTTPPanel(endpoints)
	case "git":
		return NewGitPanel("")
	default:
		// Placeholder for unknown panel types
		return NewPlaceholderPanel(name)
	}
}

// Title returns the tab title
func (d *DashboardTab) Title() string {
	return d.title
}

// Update handles updates for the tab
func (d *DashboardTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle specific messages
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Handle window resize
		d.SetSize(msg.Width, msg.Height)
		// Don't forward resize to children here - they'll get it from SetSize
		return d, nil
	}

	// Update all panels in the layout
	for _, row := range d.layout.rows {
		for i, panel := range row.panels {
			if teaPanel, ok := panel.(tea.Model); ok {
				updatedPanel, cmd := teaPanel.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}

				// Update the panel in the layout
				if updatedTeaPanel, ok := updatedPanel.(Panel); ok {
					row.panels[i] = updatedTeaPanel
				}
			} else if updatablePanel, ok := panel.(RenderablePanel); ok {
				// For panels that implement RenderablePanel, call Update
				updatablePanel.Update()
			}
			// Skip updating for panels that don't implement either interface
		}
	}

	if len(cmds) > 0 {
		return d, tea.Batch(cmds...)
	}
	return d, nil
}

// View renders the tab
func (d *DashboardTab) View() string {
	// Render the layout
	content := d.layout.Render()

	// Style the content with the app base style
	return Theme.App.Render(content)
}

// SetSize sets the tab size
func (d *DashboardTab) SetSize(width, height int) {
	// Skip resizing if dimensions haven't changed
	if d.width == width && d.height == height {
		return
	}

	// Update our dimensions
	d.width = width
	d.height = height

	// Ensure minimum dimensions
	width = MaxInt(width, 80)
	height = MaxInt(height, 24)

	// Update layout dimensions
	d.layout.SetSize(width, height)

	// If we have any components to resize, do it here
	for _, component := range d.components {
		if sizeComponent, ok := component.(interface{ SetSize(int, int) }); ok {
			// Resize components to fill available space
			sizeComponent.SetSize(width, height)
		}
	}
}

// Close properly cleans up all panels and resources
func (d *DashboardTab) Close() {
	// Close any panels that need to be cleaned up
	for _, row := range d.layout.rows {
		for _, panel := range row.panels {
			// If the panel implements a Closeable interface, close it
			if closeable, ok := panel.(interface{ Close() }); ok {
				closeable.Close()
			}
		}
	}
}

// Closeable is an interface for panels that need cleanup when closed
type Closeable interface {
	Close()
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

// Close cleans up panel resources
func (p *PlaceholderPanel) Close() {
	// No resources to clean up for a placeholder
}
