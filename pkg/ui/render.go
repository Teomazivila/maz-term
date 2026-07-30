package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// render draws one complete frame.
//
// This is the step the dashboard previously lacked: the event loop refreshed
// widget state and recomputed rectangles, but never called ui.Render on any
// dashboard widget, so the terminal stayed blank and every layout function was
// unreachable.
//
// Everything is drawn in a single ui.Render call. Rendering the widgets and then
// separately rendering a grid containing the same widgets drew each one twice
// per frame.
func (a *App) render() {
	a.TermWidth, a.TermHeight = ui.TerminalDimensions()

	ui.Clear()

	if a.terminalTooSmall() {
		a.renderTooSmall()
		return
	}

	ui.Render(a.buildFrame()...)

	// Overlays are rendered after the frame so they sit on top of it.
	if a.ShowHelp {
		a.renderOverlay(a.HelpPanel)
	}
	if a.annotationFormVisible() {
		a.renderOverlay(a.AnnotationForm)
	}
}

// buildFrame positions everything that makes up one frame and returns it in draw
// order: tab content first, then the status and tab bars.
//
// It is separate from render so the frame can be asserted on without a terminal,
// which is what makes the "nothing was ever drawn" defect testable.
func (a *App) buildFrame() []ui.Drawable {
	drawables := make([]ui.Drawable, 0, 16)

	if tab := a.activeTab(); tab != nil {
		drawables = append(drawables, a.layoutTab(tab, a.contentRect())...)
	}

	if a.StatusBar != nil {
		drawables = append(drawables, a.renderStatusBar())
	}
	if a.TabBar != nil {
		drawables = append(drawables, a.renderTabBar())
	}

	return drawables
}

// terminalTooSmall reports whether the window can fit the dashboard chrome.
func (a *App) terminalTooSmall() bool {
	return a.TermWidth < minContentWidth ||
		a.TermHeight < minContentHeight+statusBarHeight+tabBarHeight
}

// renderTooSmall replaces the frame with a notice when the terminal cannot fit
// the dashboard, rather than laying out into a degenerate rectangle.
func (a *App) renderTooSmall() {
	notice := widgets.NewParagraph()
	notice.Title = "maz-term"
	notice.Text = fmt.Sprintf(
		"Terminal too small (%dx%d).\nResize to at least %dx%d.",
		a.TermWidth, a.TermHeight,
		minContentWidth, minContentHeight+statusBarHeight+tabBarHeight,
	)
	notice.WrapText = true
	notice.SetRect(0, 0, max(a.TermWidth, 1), max(a.TermHeight, 1))
	ui.Render(notice)
}

// renderStatusBar positions and fills the status bar.
func (a *App) renderStatusBar() ui.Drawable {
	top := a.TermHeight - statusBarHeight - tabBarHeight
	a.StatusBar.SetRect(0, top, a.TermWidth, a.TermHeight-tabBarHeight)

	segments := []string{
		"maz-term",
		time.Now().Format("15:04:05"),
	}

	if a.activeTabName() == "History" {
		segments = append(segments, "range "+historyRanges[a.clampedRangeIndex()].Label)
		if a.ZoomMode {
			segments = append(segments, "ZOOM")
		}
	}

	if a.statusMessage != "" {
		segments = append(segments, a.statusMessage)
	}

	a.StatusBar.Text = strings.Join(segments, " | ")
	return a.StatusBar
}

// renderTabBar positions and fills the tab bar.
func (a *App) renderTabBar() ui.Drawable {
	a.TabBar.SetRect(0, a.TermHeight-tabBarHeight, a.TermWidth, a.TermHeight)
	a.TabBar.TabNames = a.getTabNames()
	a.TabBar.ActiveTabIndex = a.ActiveTabIndex
	return a.TabBar
}

// renderOverlay centres a panel over the content area and draws it.
func (a *App) renderOverlay(panel *widgets.Paragraph) {
	if panel == nil {
		return
	}

	content := a.contentRect()
	inset := func(v, by, min int) int {
		if v-2*by < min {
			return 0
		}
		return by
	}

	dx := inset(content.Dx(), content.Dx()/6, 40)
	dy := inset(content.Dy(), content.Dy()/8, 12)

	panel.SetRect(content.Min.X+dx, content.Min.Y+dy, content.Max.X-dx, content.Max.Y-dy)
	ui.Render(panel)
}

// clampedRangeIndex returns HistoryRangeIdx constrained to historyRanges, so a
// zoom-applied custom range cannot index out of bounds.
func (a *App) clampedRangeIndex() int {
	if a.HistoryRangeIdx < 0 {
		return 0
	}
	if a.HistoryRangeIdx >= len(historyRanges) {
		return len(historyRanges) - 1
	}
	return a.HistoryRangeIdx
}

// annotationFormVisible reports whether the annotation form should be drawn.
func (a *App) annotationFormVisible() bool {
	return a.AnnotationForm != nil && a.annotation.Field >= 0 && a.addingAnnotation
}

// rowSpec is one row in a stack: either a fixed height or a share of the
// leftover space.
type rowSpec struct {
	fixed  int
	weight int
}

// fixedRows requests exactly n rows.
func fixedRows(n int) rowSpec { return rowSpec{fixed: n} }

// flexRows requests a share of whatever height remains.
func flexRows(weight int) rowSpec { return rowSpec{weight: max(weight, 1)} }

// stackRows divides r vertically, honouring fixed heights first and sharing the
// remainder between the flexible rows.
//
// Proportional splits alone made small widgets grow absurdly on a tall terminal:
// a gauge given 18% of 40 rows became a seven-row block of solid colour.
func stackRows(rect image.Rectangle, specs ...rowSpec) []image.Rectangle {
	out := make([]image.Rectangle, len(specs))
	if len(specs) == 0 || rect.Dy() <= 0 {
		return out
	}

	remaining := rect.Dy()
	totalWeight := 0
	for _, spec := range specs {
		if spec.fixed > 0 {
			remaining -= spec.fixed
		} else {
			totalWeight += spec.weight
		}
	}
	if remaining < 0 {
		remaining = 0
	}

	heights := make([]int, len(specs))
	granted, lastFlex := 0, -1
	for i, spec := range specs {
		if spec.fixed > 0 {
			heights[i] = spec.fixed
			continue
		}
		lastFlex = i
		if totalWeight > 0 {
			heights[i] = remaining * spec.weight / totalWeight
			granted += heights[i]
		}
	}
	if lastFlex >= 0 {
		heights[lastFlex] += remaining - granted
	}

	y := rect.Min.Y
	for i, height := range heights {
		if height < 0 {
			height = 0
		}
		bottom := min(y+height, rect.Max.Y)
		out[i] = image.Rect(rect.Min.X, y, rect.Max.X, bottom)
		y = bottom
	}

	return out
}

// splitRows divides r vertically in proportion to weights. It always returns
// len(weights) rectangles; those that do not fit are empty, so callers can index
// the result unconditionally.
func splitRows(r image.Rectangle, weights ...float64) []image.Rectangle {
	return split(r, true, weights...)
}

// splitCols divides r horizontally in proportion to weights.
func splitCols(r image.Rectangle, weights ...float64) []image.Rectangle {
	return split(r, false, weights...)
}

func split(r image.Rectangle, vertical bool, weights ...float64) []image.Rectangle {
	out := make([]image.Rectangle, len(weights))
	if len(weights) == 0 {
		return out
	}

	total := 0.0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return out
	}

	extent := r.Dy()
	if !vertical {
		extent = r.Dx()
	}
	if extent <= 0 {
		return out
	}

	offset := 0
	for i, w := range weights {
		size := 0
		if w > 0 {
			size = int(float64(extent) * w / total)
		}
		// The final section absorbs the rounding remainder so the split covers
		// the whole rectangle exactly.
		if i == len(weights)-1 {
			size = extent - offset
		}
		if size < 0 {
			size = 0
		}

		if vertical {
			out[i] = image.Rect(r.Min.X, r.Min.Y+offset, r.Max.X, r.Min.Y+offset+size)
		} else {
			out[i] = image.Rect(r.Min.X+offset, r.Min.Y, r.Min.X+offset+size, r.Max.Y)
		}

		offset += size
		if offset >= extent {
			offset = extent
		}
	}

	return out
}

// minWidgetExtent is the smallest rectangle a bordered widget can occupy and
// still show anything between its borders.
const minWidgetExtent = 3

// place positions widget at rect and returns it for rendering. A widget too small
// to show content is dropped rather than drawn as a sliver of border.
func place(widget ui.Drawable, rect image.Rectangle) ui.Drawable {
	if rect.Dx() < minWidgetExtent || rect.Dy() < minWidgetExtent {
		return nil
	}
	widget.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)
	return widget
}

// compact positions each widget at the matching rect, skipping any widget that
// is missing or does not fit. It is the single guard that keeps a tab with fewer
// widgets than its layout expects from panicking on an out-of-range index.
func compact(widgets []ui.Drawable, rects []image.Rectangle) []ui.Drawable {
	out := make([]ui.Drawable, 0, len(rects))
	for i, rect := range rects {
		if i >= len(widgets) || widgets[i] == nil {
			continue
		}
		if positioned := place(widgets[i], rect); positioned != nil {
			out = append(out, positioned)
		}
	}
	return out
}
