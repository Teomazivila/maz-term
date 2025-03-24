package ui

import (
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
			// Create the appropriate panel based on the name
			panel := createPanel(panelName)
			panels = append(panels, panel)
		}

		// Add the row to the layout
		tab.layout.AddRow(row.Size, panels...)
	}

	return tab
}

// createPanel creates a panel based on its name
func createPanel(name string) Panel {
	switch name {
	case "system":
		return NewSystemPanel()
	case "git":
		return NewGitPanel("") // Current directory
	case "http":
		// Sample endpoints for demonstration
		endpoints := []models.EndpointConfig{
			{
				Name:   "Google",
				URL:    "https://www.google.com",
				Method: "GET",
			},
			{
				Name:   "GitHub",
				URL:    "https://api.github.com",
				Method: "GET",
			},
			{
				Name:           "Example",
				URL:            "https://example.com",
				Method:         "GET",
				ExpectedStatus: 200,
			},
		}
		return NewHTTPPanel(endpoints)
	default:
		// Fallback to placeholder for unknown panel types
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

	return d, tea.Batch(cmds...)
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
