package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout manages the positioning and rendering of UI components
type Layout struct {
	width   int
	height  int
	rows    []LayoutRow
	spacing int
}

// Panel represents a UI panel in the layout
type Panel interface {
	// Title returns the title of the panel
	Title() string
}

// RenderablePanel is a panel that implements its own rendering logic
type RenderablePanel interface {
	Panel

	// Render renders the panel with the given width and height
	Render(width, height int) string

	// Update updates the panel's state
	Update()
}

// SetSizeAwarePanel is a panel that can be informed of size changes
type SetSizeAwarePanel interface {
	Panel

	// SetSize sets the panel's size
	SetSize(width, height int)
}

// ViewablePanel is a panel that implements the View method for tea.Model
type ViewablePanel interface {
	Panel

	// View returns the panel's rendered view
	View() string
}

// LayoutRow represents a row in the layout
type LayoutRow struct {
	size   int
	panels []Panel
}

// NewLayout creates a new layout manager
func NewLayout(spacing int) *Layout {
	return &Layout{
		rows:    []LayoutRow{},
		spacing: spacing,
	}
}

// AddRow adds a new row to the layout
func (l *Layout) AddRow(size int, panels ...Panel) {
	l.rows = append(l.rows, LayoutRow{
		size:   size,
		panels: panels,
	})
}

// SetSize sets the size of the layout
func (l *Layout) SetSize(width, height int) {
	l.width = width
	l.height = height

	// Calculate the total weight of all rows
	totalWeight := 0
	for _, row := range l.rows {
		totalWeight += row.size
	}

	// If no rows, return
	if totalWeight == 0 {
		return
	}

	// Calculate the available height (excluding spacing)
	availableHeight := l.height - ((len(l.rows) - 1) * l.spacing)

	// Set size for each row's panels
	for _, row := range l.rows {
		// Calculate the height for this row
		rowHeight := (row.size * availableHeight) / totalWeight

		// Calculate available width for panels (excluding spacing)
		availableWidth := l.width - ((len(row.panels) - 1) * l.spacing)

		// Set size for each panel
		for j, panel := range row.panels {
			// Calculate panel width (equal distribution for now)
			panelWidth := availableWidth / len(row.panels)

			// For the last panel, use remaining width
			if j == len(row.panels)-1 {
				panelWidth = availableWidth - (panelWidth * (len(row.panels) - 1))
			}

			// Set panel size if it supports it
			if sizePanel, ok := panel.(SetSizeAwarePanel); ok {
				sizePanel.SetSize(panelWidth, rowHeight)
			}
		}
	}
}

// Render renders the layout
func (l *Layout) Render() string {
	// Calculate the total weight of all rows
	totalWeight := 0
	for _, row := range l.rows {
		totalWeight += row.size
	}

	// If no rows, return empty string
	if totalWeight == 0 {
		return ""
	}

	// Calculate the available height (excluding spacing)
	availableHeight := l.height - ((len(l.rows) - 1) * l.spacing)

	// Ensure minimum available height
	availableHeight = MaxInt(availableHeight, 20) // Minimum 20 lines

	// Render each row
	renderedRows := make([]string, len(l.rows))
	for i, row := range l.rows {
		// Calculate the height for this row
		rowHeight := (row.size * availableHeight) / totalWeight

		// Ensure minimum row height
		rowHeight = MaxInt(rowHeight, 10) // Minimum 10 lines per row

		// Render the row
		renderedRows[i] = l.renderRow(row, rowHeight)
	}

	// Join all rows with spacing
	return lipgloss.JoinVertical(lipgloss.Left, renderedRows...)
}

// renderRow renders a single row of panels
func (l *Layout) renderRow(row LayoutRow, height int) string {
	// If no panels, return empty string
	if len(row.panels) == 0 {
		return strings.Repeat(" ", l.width)
	}

	// Calculate available width for panels (excluding spacing)
	availableWidth := l.width - ((len(row.panels) - 1) * l.spacing)

	// Ensure minimum width
	availableWidth = MaxInt(availableWidth, 80) // Minimum 80 columns

	// Calculate panel width (equal distribution for now)
	panelWidth := availableWidth / len(row.panels)

	// Ensure minimum panel width
	panelWidth = MaxInt(panelWidth, 40) // Minimum 40 columns per panel

	// Render each panel
	renderedPanels := make([]string, len(row.panels))
	for i, panel := range row.panels {
		// For the last panel, use remaining width
		width := panelWidth
		if i == len(row.panels)-1 {
			width = availableWidth - (panelWidth * (len(row.panels) - 1))
		}

		// Render the panel
		renderedPanels[i] = l.renderPanel(panel, width, height)
	}

	// Join all panels horizontally with spacing
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedPanels...)
}

// renderPanel renders a single panel
func (l *Layout) renderPanel(panel Panel, width, height int) string {
	var content string
	title := panel.Title()

	// Set minimum dimensions to ensure panels are visible even with small terminal sizes
	width = MaxInt(width, 30)   // Minimum width
	height = MaxInt(height, 10) // Minimum height

	// Style for the panel border and title
	panelStyle := Theme.Panel.Copy().
		Width(width).
		Height(height)

	// Check if panel implements ViewablePanel (tea.Model)
	if viewable, ok := panel.(ViewablePanel); ok {
		content = viewable.View()
	} else if renderable, ok := panel.(RenderablePanel); ok {
		// Legacy panel with Render method
		content = renderable.Render(width-4, height-4) // Account for padding and borders
	} else {
		// Fallback for panels that don't implement either interface
		content = "Panel does not implement View or Render"
	}

	// Render the panel with title and content
	styledTitle := Theme.PanelTitle.Render(title)
	styledPanel := panelStyle.Render(styledTitle + "\n" + content)

	return styledPanel
}

// Update updates all panels in the layout
func (l *Layout) Update() {
	for _, row := range l.rows {
		for _, panel := range row.panels {
			// Use Update() for legacy panels
			if updatable, ok := panel.(RenderablePanel); ok {
				updatable.Update()
			}
		}
	}
}

// GetRows returns the rows in the layout (for iterating when closing)
func (l *Layout) GetRows() []LayoutRow {
	return l.rows
}

// GetPanels returns the panels in a row
func (r *LayoutRow) GetPanels() []Panel {
	return r.panels
}

// max returns the maximum of two integers
