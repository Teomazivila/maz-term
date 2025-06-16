package ui

import (
	"image"

	ui "github.com/gizak/termui/v3"
)

// layoutTab arranges the widgets for a given tab inside the provided rectangle
func (a *App) layoutTab(tab *Tab, rect image.Rectangle) {
	// Skip if tab has no widgets
	if len(tab.Widgets) == 0 {
		return
	}

	// Determine layout based on tab name
	switch tab.Name {
	case "System":
		a.layoutSystemTab(tab, rect)
	case "HTTP":
		a.layoutHTTPTab(tab, rect)
	case "Git":
		a.layoutGitTab(tab, rect)
	case "Cloud":
		a.layoutCloudTab(tab, rect)
	case "Kubernetes":
		a.layoutKubernetesTab(tab, rect)
	case "CI/CD":
		a.layoutCICDTab(tab, rect)
	case "History":
		a.layoutHistoryTab(tab, rect)
	case "Notifications":
		a.layoutNotificationsTab(tab, rect)
	case "Plugins":
		a.layoutPluginsTab(tab, rect)
	default:
		// Default layout: arrange widgets in grid
		a.layoutDefaultTab(tab, rect)
	}

	// Render widgets
	for _, widget := range tab.Widgets {
		ui.Render(widget)
	}
}

// layoutSystemTab arranges the System tab widgets
func (a *App) layoutSystemTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Set up custom grid layout based on layout config if available
	if a.Config != nil {
		for _, tabCfg := range a.Config.Layout {
			if tabCfg.Name == "System" {
				// If a custom layout is defined, use it
				if len(tabCfg.Layout) > 0 {
					// Parse layout from config
					gridRows := make([][]int, len(tabCfg.Layout))
					for i, row := range tabCfg.Layout {
						gridRows[i] = row
					}
					grid.Set(gridRows)
					break
				}
			}
		}
	}

	// Default system tab layout if no custom layout defined
	if len(grid.Items) == 0 {
		grid.Set(
			ui.NewRow(0.2,
				ui.NewCol(0.5, tab.Widgets[0]), // CPU gauge
				ui.NewCol(0.5, tab.Widgets[1]), // Memory gauge
			),
			ui.NewRow(0.2, tab.Widgets[2]), // CPU sparkline
			ui.NewRow(0.2, tab.Widgets[3]), // Disk chart
			ui.NewRow(0.4, tab.Widgets[4]), // Process table
		)
	}

	// Render the grid
	ui.Render(grid)
}

// layoutHTTPTab arranges the HTTP tab widgets
func (a *App) layoutHTTPTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Default HTTP tab layout
	grid.Set(
		ui.NewRow(0.4, tab.Widgets[0]), // Endpoints table
		ui.NewRow(0.2,
			ui.NewCol(0.5, tab.Widgets[1]), // Response time sparkline
			ui.NewCol(0.5, tab.Widgets[2]), // Availability sparkline
		),
		ui.NewRow(0.4, tab.Widgets[3]), // Details panel
	)

	// Render the grid
	ui.Render(grid)
}

// layoutGitTab arranges the Git tab widgets
func (a *App) layoutGitTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Default Git tab layout
	grid.Set(
		ui.NewRow(0.2, tab.Widgets[0]), // Repository status
		ui.NewRow(0.4, tab.Widgets[1]), // Changes table
		ui.NewRow(0.3, tab.Widgets[2]), // Commit history
		ui.NewRow(0.1, tab.Widgets[3]), // Branch list
	)

	// Render the grid
	ui.Render(grid)
}

// layoutHistoryTab arranges the History tab widgets
func (a *App) layoutHistoryTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Default History tab layout depends on the number of plots
	if len(tab.Plots) == 1 {
		// Only one plot
		grid.Set(
			ui.NewRow(1.0, tab.Widgets[0]), // Single plot
		)
	} else if len(tab.Plots) == 2 {
		// Two plots
		grid.Set(
			ui.NewRow(0.5, tab.Widgets[0]), // First plot
			ui.NewRow(0.5, tab.Widgets[1]), // Second plot
		)
	} else if len(tab.Plots) == 3 {
		// Three plots
		grid.Set(
			ui.NewRow(0.33, tab.Widgets[0]), // First plot
			ui.NewRow(0.33, tab.Widgets[1]), // Second plot
			ui.NewRow(0.34, tab.Widgets[2]), // Third plot
		)
	} else if len(tab.Plots) == 4 {
		// Four plots
		grid.Set(
			ui.NewRow(0.5,
				ui.NewCol(0.5, tab.Widgets[0]), // First plot
				ui.NewCol(0.5, tab.Widgets[1]), // Second plot
			),
			ui.NewRow(0.5,
				ui.NewCol(0.5, tab.Widgets[2]), // Third plot
				ui.NewCol(0.5, tab.Widgets[3]), // Fourth plot
			),
		)
	} else if len(tab.Plots) > 4 {
		// More than four plots - 2x3 grid
		grid.Set(
			ui.NewRow(0.33,
				ui.NewCol(0.5, tab.Widgets[0]), // First plot
				ui.NewCol(0.5, tab.Widgets[1]), // Second plot
			),
			ui.NewRow(0.33,
				ui.NewCol(0.5, tab.Widgets[2]), // Third plot
				ui.NewCol(0.5, tab.Widgets[3]), // Fourth plot
			),
			ui.NewRow(0.34,
				ui.NewCol(0.5, tab.Widgets[4]),                // Fifth plot
				ui.NewCol(0.5, tab.Widgets[5%len(tab.Plots)]), // Sixth plot or wrap around
			),
		)
	}

	// Render the grid
	ui.Render(grid)
}

// layoutNotificationsTab arranges the Notifications tab widgets
func (a *App) layoutNotificationsTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Default Notifications tab layout
	if a.NotificationDetailMode {
		// When viewing notification details, show details panel larger
		grid.Set(
			ui.NewRow(0.3, tab.Widgets[0]), // Notifications list
			ui.NewRow(0.6, tab.Widgets[1]), // Notification details
			ui.NewRow(0.1, tab.Widgets[2]), // Filter display
		)
	} else if a.NotificationFilterMode {
		// When in filter mode, show filter panel larger
		grid.Set(
			ui.NewRow(0.3, tab.Widgets[0]), // Notifications list
			ui.NewRow(0.2, tab.Widgets[1]), // Notification details
			ui.NewRow(0.5, tab.Widgets[2]), // Filter display
		)
	} else {
		// Normal layout
		grid.Set(
			ui.NewRow(0.6, tab.Widgets[0]), // Notifications list
			ui.NewRow(0.3, tab.Widgets[1]), // Notification details
			ui.NewRow(0.1, tab.Widgets[2]), // Filter display
		)
	}

	// Render the grid
	ui.Render(grid)
}

// layoutPluginsTab arranges the Plugins tab widgets
func (a *App) layoutPluginsTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Default Plugins tab layout
	grid.Set(
		ui.NewRow(0.4, tab.Widgets[0]), // Plugin list
		ui.NewRow(0.6, tab.Widgets[1]), // Plugin details
	)

	// Render the grid
	ui.Render(grid)
}

// layoutDefaultTab arranges widgets in a default grid layout
func (a *App) layoutDefaultTab(tab *Tab, rect image.Rectangle) {
	// Create a grid for this tab
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Default to a simple stack layout
	if len(tab.Widgets) == 1 {
		// Just one widget - fill the space
		grid.Set(
			ui.NewRow(1.0, tab.Widgets[0]),
		)
	} else if len(tab.Widgets) == 2 {
		// Two widgets - split vertically
		grid.Set(
			ui.NewRow(0.5, tab.Widgets[0]),
			ui.NewRow(0.5, tab.Widgets[1]),
		)
	} else if len(tab.Widgets) == 3 {
		// Three widgets - split vertically
		grid.Set(
			ui.NewRow(0.33, tab.Widgets[0]),
			ui.NewRow(0.33, tab.Widgets[1]),
			ui.NewRow(0.34, tab.Widgets[2]),
		)
	} else if len(tab.Widgets) == 4 {
		// Four widgets - 2x2 grid
		grid.Set(
			ui.NewRow(0.5,
				ui.NewCol(0.5, tab.Widgets[0]),
				ui.NewCol(0.5, tab.Widgets[1]),
			),
			ui.NewRow(0.5,
				ui.NewCol(0.5, tab.Widgets[2]),
				ui.NewCol(0.5, tab.Widgets[3]),
			),
		)
	} else {
		// More than four widgets - create rows with 2 columns each
		rows := (len(tab.Widgets) + 1) / 2 // Ceiling division
		rowHeight := 1.0 / float64(rows)

		gridRows := make([]interface{}, rows)
		for i := 0; i < rows; i++ {
			// Last row might have only one widget
			if i == rows-1 && len(tab.Widgets)%2 == 1 {
				gridRows[i] = ui.NewRow(rowHeight, tab.Widgets[i*2])
			} else {
				gridRows[i] = ui.NewRow(rowHeight,
					ui.NewCol(0.5, tab.Widgets[i*2]),
					ui.NewCol(0.5, tab.Widgets[i*2+1]),
				)
			}
		}

		grid.Set(gridRows...)
	}

	// Render the grid
	ui.Render(grid)
}

// layoutCloudTab arranges the Cloud tab widgets
func (a *App) layoutCloudTab(tab *Tab, rect image.Rectangle) {
	// Example layout - would be customized based on actual cloud widgets
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Simple layout for now
	if len(tab.Widgets) > 0 {
		grid.Set(
			ui.NewRow(1.0, tab.Widgets[0]), // Main cloud widget
		)
	}

	// Render the grid
	ui.Render(grid)
}

// layoutKubernetesTab arranges the Kubernetes tab widgets
func (a *App) layoutKubernetesTab(tab *Tab, rect image.Rectangle) {
	// Example layout - would be customized based on actual Kubernetes widgets
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Simple layout for now
	if len(tab.Widgets) > 0 {
		grid.Set(
			ui.NewRow(1.0, tab.Widgets[0]), // Main Kubernetes widget
		)
	}

	// Render the grid
	ui.Render(grid)
}

// layoutCICDTab arranges the CI/CD tab widgets
func (a *App) layoutCICDTab(tab *Tab, rect image.Rectangle) {
	// Example layout - would be customized based on actual CI/CD widgets
	grid := ui.NewGrid()
	grid.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	// Simple layout for now
	if len(tab.Widgets) > 0 {
		grid.Set(
			ui.NewRow(1.0, tab.Widgets[0]), // Main CI/CD widget
		)
	}

	// Render the grid
	ui.Render(grid)
}
