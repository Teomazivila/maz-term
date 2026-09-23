package ui

import (
	"fmt"
	"image"

	ui "github.com/gizak/termui/v3"
)

// yAxisLabelWidth is reserved for the value labels down the left edge.
const yAxisLabelWidth = 7

// Sparkline plots a series as vertical bars with sub-cell precision.
//
// termui's own sparkline draws a single row of eighth-height runes, so a series
// spanning 0-100 is compressed into eight distinguishable levels and reads as a
// solid strip. This widget spreads the series over the full height of its box and
// still uses the eighth-height runes for the top of each bar, giving eight times
// the resolution of a whole-cell bar chart at whatever height it is given.
//
// The newest sample sits at the right edge, so the series grows leftwards the way
// a monitoring chart is read.
type Sparkline struct {
	*ui.Block

	// Data is ordered oldest to newest.
	Data []float64

	// Max fixes the top of the scale. Zero scales to the series, rounded up to a
	// round number.
	Max float64

	// Unit is appended to the values in the axis and legend, for example "%".
	Unit string

	// Label prefixes the legend.
	Label string

	LineColor ui.Color
	AxisColor ui.Color
	TextColor ui.Color

	// ShowAxis draws the value axis when there is room for it.
	ShowAxis bool

	// ShowLegend draws the current, mean and peak values under the plot.
	ShowLegend bool

	// ColorByThreshold grades the bars green, amber then red by percentage, for
	// series that are percentages.
	ColorByThreshold bool
}

// NewSparkline creates a sparkline.
func NewSparkline(title string) *Sparkline {
	block := ui.NewBlock()
	block.Title = title
	block.BorderStyle.Fg = ui.ColorCyan

	return &Sparkline{
		Block:      block,
		LineColor:  ui.ColorGreen,
		AxisColor:  ui.ColorClear,
		TextColor:  ui.ColorWhite,
		ShowAxis:   true,
		ShowLegend: true,
	}
}

// Append adds a sample, discarding the oldest beyond limit.
func (s *Sparkline) Append(value float64, limit int) {
	s.Data = append(s.Data, value)
	if limit > 0 && len(s.Data) > limit {
		s.Data = s.Data[len(s.Data)-limit:]
	}
}

// scaleMax returns the top of the value axis.
func (s *Sparkline) scaleMax() float64 {
	if s.Max > 0 {
		return s.Max
	}

	stats := statsOf(s.Data)
	if stats.Max <= 0 {
		return 1
	}
	return niceMax(stats.Max)
}

// Draw renders the sparkline.
func (s *Sparkline) Draw(buf *ui.Buffer) {
	s.Block.Draw(buf)

	inner := s.Inner
	if inner.Dx() < 2 || inner.Dy() < 1 {
		return
	}

	axisStyle := ui.NewStyle(s.AxisColor)
	textStyle := ui.NewStyle(s.TextColor)

	// The legend takes the bottom row when there is height to spare for it.
	plot := inner
	legendRow := -1
	if s.ShowLegend && inner.Dy() >= 3 {
		legendRow = inner.Max.Y - 1
		plot.Max.Y = legendRow
	}

	// The axis takes a strip on the left when there is width to spare.
	withAxis := s.ShowAxis && plot.Dx() > yAxisLabelWidth+8 && plot.Dy() >= 2
	if withAxis {
		s.drawAxis(buf, plot, axisStyle, textStyle)
		plot.Min.X += yAxisLabelWidth + 1
	}

	if plot.Dx() > 0 && plot.Dy() > 0 {
		s.drawSeries(buf, plot)
	}

	if legendRow >= 0 {
		s.drawLegend(buf, inner, legendRow, textStyle)
	}
}

// drawAxis writes the value axis and its labels.
func (s *Sparkline) drawAxis(buf *ui.Buffer, plot image.Rectangle, axisStyle, textStyle ui.Style) {
	max := s.scaleMax()
	axisX := plot.Min.X + yAxisLabelWidth

	drawYAxis(buf, plot, axisX, axisStyle)

	// Only the extremes are labelled: intermediate ticks cost rows that the plot
	// needs more, and the shape carries the detail.
	top := fmt.Sprintf("%*s", yAxisLabelWidth, formatMetric(max, s.Unit))
	putString(buf, plot, top, image.Pt(plot.Min.X, plot.Min.Y), textStyle)

	if plot.Dy() >= 3 {
		mid := fmt.Sprintf("%*s", yAxisLabelWidth, formatMetric(max/2, s.Unit))
		putString(buf, plot, mid, image.Pt(plot.Min.X, plot.Min.Y+plot.Dy()/2), textStyle)
	}

	bottom := fmt.Sprintf("%*s", yAxisLabelWidth, "0")
	putString(buf, plot, bottom, image.Pt(plot.Min.X, plot.Max.Y-1), textStyle)
}

// drawSeries plots the bars, newest at the right edge.
func (s *Sparkline) drawSeries(buf *ui.Buffer, plot image.Rectangle) {
	if len(s.Data) == 0 {
		return
	}

	max := s.scaleMax()
	width := plot.Dx()

	// Show the most recent samples that fit; older ones scroll off the left.
	data := s.Data
	if len(data) > width {
		data = data[len(data)-width:]
	}

	// Right-align so the newest sample is always at the right edge, which keeps
	// the chart stable as the series fills up.
	x := plot.Max.X - len(data)

	for _, value := range data {
		eighths := scaleToEighths(value, max, plot.Dy())

		color := s.LineColor
		if s.ColorByThreshold {
			color = thresholdColor(value)
		}

		drawVerticalBar(buf, plot, x, 1, eighths, ui.NewStyle(color))
		x++
	}
}

// drawLegend writes the current, mean and peak values.
func (s *Sparkline) drawLegend(buf *ui.Buffer, inner image.Rectangle, y int, style ui.Style) {
	stats := statsOf(s.Data)

	legend := "collecting…"
	if stats.Count > 0 {
		legend = fmt.Sprintf("now %s   avg %s   peak %s   n=%d",
			formatMetric(stats.Current, s.Unit),
			formatMetric(stats.Mean, s.Unit),
			formatMetric(stats.Max, s.Unit),
			stats.Count)
	}
	if s.Label != "" {
		legend = s.Label + "   " + legend
	}

	putString(buf, inner, fitCell(legend, inner.Dx(), AlignLeft), image.Pt(inner.Min.X, y), style)
}
