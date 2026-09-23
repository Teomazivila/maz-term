package ui

import (
	"fmt"
	"image"
	"math"

	ui "github.com/gizak/termui/v3"
)

// Rune sets for sub-cell rendering.
//
// A terminal cell is the smallest addressable unit, so a bar drawn only with a
// full block can be no more precise than one whole row. These eighth-height and
// eighth-width runes give eight times that resolution, which is the difference
// between a chart that reads as a smooth curve and one that reads as a stack of
// solid rectangles.
//
// They are declared here rather than taken from termui because termui swaps parts
// of its symbol table on Windows, and a chart should render identically
// everywhere.
var (
	// verticalBlocks fill a cell from the bottom up, in eighths.
	verticalBlocks = [8]rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// horizontalBlocks fill a cell from the left, in eighths.
	horizontalBlocks = [8]rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}
)

// Box-drawing runes used for axes. Light weight reads as a guide rather than
// competing with the data.
const (
	axisVertical    = '│'
	axisHorizontal  = '─'
	axisCorner      = '└'
	axisTick        = '┼'
	fullBlock       = '█'
	subCellsPerCell = 8
)

// subCell splits a value, already expressed in eighths, into whole cells plus the
// partial rune that tops them off.
//
// This is the core of k9s's chart rendering: draw full blocks for the whole cells
// and one eighth-height rune for the remainder.
func subCell(eighths int) (full int, partial rune) {
	if eighths <= 0 {
		return 0, 0
	}

	full = eighths / subCellsPerCell
	if remainder := eighths % subCellsPerCell; remainder > 0 {
		partial = verticalBlocks[remainder-1]
	}
	return full, partial
}

// scaleToEighths converts a value to eighths of a cell, given the plot height and
// the maximum value the axis represents.
func scaleToEighths(value, max float64, rows int) int {
	if max <= 0 || rows <= 0 || value <= 0 {
		return 0
	}

	eighths := int(math.Round(value / max * float64(rows*subCellsPerCell)))
	return min(eighths, rows*subCellsPerCell)
}

// drawVerticalBar draws one bar of the given width, rising from the bottom of
// rect, filled to eighths.
func drawVerticalBar(buf *ui.Buffer, rect image.Rectangle, x, width, eighths int, style ui.Style) {
	full, partial := subCell(eighths)

	for column := x; column < x+width && column < rect.Max.X; column++ {
		y := rect.Max.Y - 1

		for range full {
			if y < rect.Min.Y {
				break
			}
			buf.SetCell(ui.NewCell(fullBlock, style), image.Pt(column, y))
			y--
		}

		if partial != 0 && y >= rect.Min.Y {
			buf.SetCell(ui.NewCell(partial, style), image.Pt(column, y))
		}
	}
}

// drawHorizontalBar draws a bar growing rightwards from rect's left edge, filled
// to the given fraction, and returns the cells it occupied.
func drawHorizontalBar(buf *ui.Buffer, rect image.Rectangle, y int, fraction float64, style ui.Style) {
	if y < rect.Min.Y || y >= rect.Max.Y || rect.Dx() <= 0 {
		return
	}

	fraction = math.Max(0, math.Min(1, fraction))
	eighths := int(math.Round(fraction * float64(rect.Dx()*subCellsPerCell)))

	full := eighths / subCellsPerCell
	remainder := eighths % subCellsPerCell

	x := rect.Min.X
	for range full {
		if x >= rect.Max.X {
			return
		}
		buf.SetCell(ui.NewCell(fullBlock, style), image.Pt(x, y))
		x++
	}

	if remainder > 0 && x < rect.Max.X {
		buf.SetCell(ui.NewCell(horizontalBlocks[remainder-1], style), image.Pt(x, y))
	}
}

// drawYAxis draws a vertical rule down the left of rect, closed with a corner.
func drawYAxis(buf *ui.Buffer, rect image.Rectangle, x int, style ui.Style) {
	for y := rect.Min.Y; y < rect.Max.Y-1; y++ {
		buf.SetCell(ui.NewCell(axisVertical, style), image.Pt(x, y))
	}
	buf.SetCell(ui.NewCell(axisCorner, style), image.Pt(x, rect.Max.Y-1))
}

// drawXAxis draws a horizontal rule along the bottom of rect.
func drawXAxis(buf *ui.Buffer, rect image.Rectangle, y int, style ui.Style) {
	for x := rect.Min.X; x < rect.Max.X; x++ {
		buf.SetCell(ui.NewCell(axisHorizontal, style), image.Pt(x, y))
	}
}

// putString writes s at p, clipped to rect, and returns the number of cells used.
func putString(buf *ui.Buffer, rect image.Rectangle, s string, p image.Point, style ui.Style) int {
	used := 0
	for _, r := range s {
		x := p.X + used
		if x >= rect.Max.X || x < rect.Min.X || p.Y < rect.Min.Y || p.Y >= rect.Max.Y {
			break
		}
		// Control characters would corrupt the frame.
		if r < 0x20 || r == 0x7f {
			r = ' '
		}
		buf.SetCell(ui.NewCell(r, style), image.Pt(x, p.Y))
		used++
	}
	return used
}

// seriesStats summarises a series for a chart legend.
type seriesStats struct {
	Current float64
	Min     float64
	Max     float64
	Mean    float64
	Count   int
}

// statsOf summarises values, ignoring an empty series.
func statsOf(values []float64) seriesStats {
	if len(values) == 0 {
		return seriesStats{}
	}

	stats := seriesStats{
		Current: values[len(values)-1],
		Min:     values[0],
		Max:     values[0],
		Count:   len(values),
	}

	var sum float64
	for _, v := range values {
		stats.Min = math.Min(stats.Min, v)
		stats.Max = math.Max(stats.Max, v)
		sum += v
	}
	stats.Mean = sum / float64(len(values))

	return stats
}

// formatMetric renders a value with a unit, choosing a precision that stays
// readable across the range a metric actually takes.
func formatMetric(value float64, unit string) string {
	switch {
	case value == 0:
		return "0" + unit
	case math.Abs(value) >= 1000:
		return fmt.Sprintf("%.0f%s", value, unit)
	case math.Abs(value) >= 100:
		return fmt.Sprintf("%.1f%s", value, unit)
	default:
		return fmt.Sprintf("%.1f%s", value, unit)
	}
}

// niceMax rounds a maximum up to a round number, so the axis label reads as a
// scale rather than as whatever the highest sample happened to be.
func niceMax(max float64) float64 {
	if max <= 0 {
		return 1
	}

	magnitude := math.Pow(10, math.Floor(math.Log10(max)))
	normalised := max / magnitude

	switch {
	case normalised <= 1:
		return magnitude
	case normalised <= 2:
		return 2 * magnitude
	case normalised <= 5:
		return 5 * magnitude
	default:
		return 10 * magnitude
	}
}

// thresholdColor grades a percentage: green while healthy, amber when it warrants
// attention, red when it does not.
func thresholdColor(percent float64) ui.Color {
	switch {
	case percent >= 90:
		return ui.ColorRed
	case percent >= 75:
		return ui.ColorYellow
	default:
		return ui.ColorGreen
	}
}
