package ui

import (
	"image"

	ui "github.com/gizak/termui/v3"
)

// layoutTab positions the widgets of tab inside rect and returns them ready to
// render. It never renders directly, so the caller can draw a whole frame in one
// ui.Render call.
//
// Every layout goes through compact, which skips widgets that are absent or do
// not fit. The previous implementation guarded only the empty case and then
// indexed up to Widgets[4], panicking on any tab with fewer widgets than its
// layout assumed.
func (a *App) layoutTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	if tab == nil || len(tab.Widgets) == 0 || rect.Dx() < 2 || rect.Dy() < 2 {
		return nil
	}

	switch tab.Name {
	case "System":
		return a.layoutSystemTab(tab, rect)
	case "HTTP":
		return a.layoutHTTPTab(tab, rect)
	case "Git":
		return a.layoutGitTab(tab, rect)
	case "History":
		return a.layoutHistoryTab(tab, rect)
	case "Notifications":
		return a.layoutNotificationsTab(tab, rect)
	case "Plugins":
		return a.layoutPluginsTab(tab, rect)
	case "Cloud", "Kubernetes", "CI/CD":
		return a.layoutProviderTab(tab, rect)
	default:
		return a.layoutDefaultTab(tab, rect)
	}
}

// layoutSystemTab arranges the CPU and memory gauges, the CPU sparkline, the
// disk chart and the process table.
func (a *App) layoutSystemTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	rows := splitRows(rect, 0.18, 0.18, 0.20, 0.44)
	gauges := splitCols(rows[0], 0.5, 0.5)

	rects := []image.Rectangle{gauges[0], gauges[1], rows[1], rows[2], rows[3]}
	return compact(tab.Widgets, rects)
}

// layoutHTTPTab arranges the endpoint table, the two sparklines and the details
// panel.
func (a *App) layoutHTTPTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	rows := splitRows(rect, 0.45, 0.25, 0.30)
	sparks := splitCols(rows[1], 0.5, 0.5)

	rects := []image.Rectangle{rows[0], sparks[0], sparks[1], rows[2]}
	return compact(tab.Widgets, rects)
}

// layoutGitTab arranges the repository summary, changed files, commit history
// and branch list.
func (a *App) layoutGitTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	rows := splitRows(rect, 0.24, 0.30, 0.30, 0.16)

	rects := []image.Rectangle{rows[0], rows[1], rows[2], rows[3]}
	return compact(tab.Widgets, rects)
}

// layoutHistoryTab arranges the metric plots with the range legend beside them.
//
// The layout is driven by how many widgets the tab actually holds. The previous
// version branched on len(tab.Plots) while indexing tab.Widgets, so the two
// slices could disagree and, at exactly five plots, it rendered the first widget
// twice via Widgets[5%len(Plots)].
func (a *App) layoutHistoryTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	plotCount := len(tab.Widgets) - 1 // the final widget is the range legend
	if plotCount < 1 {
		return compact(tab.Widgets, []image.Rectangle{rect})
	}

	// The legend takes a fixed strip on the right so the plots keep a usable
	// width regardless of how many there are.
	main := rect
	var legend image.Rectangle
	if rect.Dx() > 60 {
		parts := splitCols(rect, 0.78, 0.22)
		main, legend = parts[0], parts[1]
	} else {
		parts := splitRows(rect, 0.82, 0.18)
		main, legend = parts[0], parts[1]
	}

	rects := make([]image.Rectangle, 0, plotCount+1)
	switch plotCount {
	case 1:
		rects = append(rects, main)
	case 2:
		parts := splitRows(main, 0.5, 0.5)
		rects = append(rects, parts[0], parts[1])
	case 3:
		parts := splitRows(main, 0.34, 0.33, 0.33)
		rects = append(rects, parts[0], parts[1], parts[2])
	default:
		// Two columns, as many rows as needed.
		perColumn := (plotCount + 1) / 2
		weights := make([]float64, perColumn)
		for i := range weights {
			weights[i] = 1
		}
		columns := splitCols(main, 0.5, 0.5)
		left := splitRows(columns[0], weights...)
		right := splitRows(columns[1], weights...)

		for i := 0; i < perColumn; i++ {
			rects = append(rects, left[i])
			if len(rects) < plotCount {
				rects = append(rects, right[i])
			}
		}
	}

	rects = append(rects, legend)
	return compact(tab.Widgets, rects)
}

// layoutNotificationsTab arranges the list, the detail panel and the filter
// panel, giving whichever mode is active the most room.
func (a *App) layoutNotificationsTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	var rows []image.Rectangle
	switch {
	case a.NotificationDetailMode:
		rows = splitRows(rect, 0.30, 0.58, 0.12)
	case a.NotificationFilterMode:
		rows = splitRows(rect, 0.30, 0.18, 0.52)
	default:
		rows = splitRows(rect, 0.60, 0.28, 0.12)
	}

	rects := []image.Rectangle{rows[0], rows[1], rows[2]}
	return compact(tab.Widgets, rects)
}

// layoutPluginsTab arranges the plugin list above the detail panel.
func (a *App) layoutPluginsTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	rows := splitRows(rect, 0.40, 0.60)

	rects := []image.Rectangle{rows[0], rows[1]}
	return compact(tab.Widgets, rects)
}

// layoutDefaultTab stacks widgets in one or two columns, used by any tab the
// configuration defines that the dashboard has no dedicated layout for.
func (a *App) layoutDefaultTab(tab *Tab, rect image.Rectangle) []ui.Drawable {
	count := len(tab.Widgets)

	switch count {
	case 1:
		return compact(tab.Widgets, []image.Rectangle{rect})
	case 2, 3:
		weights := make([]float64, count)
		for i := range weights {
			weights[i] = 1
		}
		return compact(tab.Widgets, splitRows(rect, weights...))
	}

	rowCount := (count + 1) / 2
	weights := make([]float64, rowCount)
	for i := range weights {
		weights[i] = 1
	}

	rowRects := splitRows(rect, weights...)
	rects := make([]image.Rectangle, 0, count)
	for i := 0; i < rowCount && len(rects) < count; i++ {
		// An odd final widget spans the full row rather than leaving a gap.
		if len(rects) == count-1 {
			rects = append(rects, rowRects[i])
			break
		}
		pair := splitCols(rowRects[i], 0.5, 0.5)
		rects = append(rects, pair[0], pair[1])
	}

	return compact(tab.Widgets, rects)
}
