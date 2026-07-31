package ui

import (
	"image"

	ui "github.com/gizak/termui/v3"
)

const (
	// minBarWidth is the narrowest a bar may be before bars are dropped instead.
	minBarWidth = 3

	// barGap separates bars.
	barGap = 1
)

// Bar is one entry in a BarChart.
type Bar struct {
	Label string
	Value float64

	// Detail replaces the rendered value when set, for cases where the raw number
	// is not the useful reading.
	Detail string

	// Color overrides the threshold grading when set. A pointer, because
	// ui.Color's zero value is ColorBlack, so a bar built as a struct literal
	// would otherwise be drawn black on black.
	Color *ui.Color
}

// BarChart draws labelled vertical bars with sub-cell precision.
//
// termui's own bar chart fills whole cells only, prints its numbers inside the
// bars where they collide with the fill, and truncates labels to a fixed width so
// long mount points ran together into an unreadable strip. This draws the value
// above each bar and the label below it, tops each bar off with an eighth-height
// rune, and drops bars that cannot be drawn legibly rather than crushing them all.
type BarChart struct {
	*ui.Block

	Bars []Bar

	// Max fixes the top of the scale. Zero scales to the data.
	Max float64

	// Unit is appended to the rendered values.
	Unit string

	TextColor ui.Color

	// ColorByThreshold grades bars green, amber then red, for percentages.
	ColorByThreshold bool

	// BarColor is the default when a bar sets none and grading is off.
	BarColor ui.Color
}

// NewBarChart creates a bar chart.
func NewBarChart(title string) *BarChart {
	block := ui.NewBlock()
	block.Title = title
	block.BorderStyle.Fg = ui.ColorCyan

	return &BarChart{
		Block:     block,
		TextColor: ui.ColorWhite,
		BarColor:  ui.ColorGreen,
	}
}

// scaleMax returns the top of the value axis.
func (b *BarChart) scaleMax() float64 {
	if b.Max > 0 {
		return b.Max
	}

	values := make([]float64, 0, len(b.Bars))
	for _, bar := range b.Bars {
		values = append(values, bar.Value)
	}

	stats := statsOf(values)
	if stats.Max <= 0 {
		return 1
	}
	return niceMax(stats.Max)
}

// barColor returns the colour for one bar.
func (b *BarChart) barColor(bar Bar) ui.Color {
	switch {
	case bar.Color != nil:
		return *bar.Color
	case b.ColorByThreshold:
		return thresholdColor(bar.Value)
	default:
		return b.BarColor
	}
}

// Draw renders the chart.
func (b *BarChart) Draw(buf *ui.Buffer) {
	b.Block.Draw(buf)

	inner := b.Inner
	if inner.Dx() < minBarWidth || inner.Dy() < 2 || len(b.Bars) == 0 {
		return
	}

	textStyle := ui.NewStyle(b.TextColor)

	// Reserve a row for the labels, and one for the values when there is height
	// for it. The bars take what is left.
	labelRow := inner.Max.Y - 1
	plot := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, labelRow)

	valueRow := -1
	if plot.Dy() >= 3 {
		valueRow = plot.Min.Y
		plot.Min.Y++
	}

	// Bars share the width evenly; any that cannot reach a legible width are
	// dropped rather than rendered as slivers.
	bars := b.Bars
	slot := (inner.Dx() + barGap) / len(bars)
	if slot < minBarWidth+barGap {
		fits := max((inner.Dx()+barGap)/(minBarWidth+barGap), 1)
		bars = bars[:min(fits, len(bars))]
		slot = (inner.Dx() + barGap) / len(bars)
	}
	barWidth := max(slot-barGap, 1)

	max := b.scaleMax()
	x := inner.Min.X

	for _, bar := range bars {
		if x >= inner.Max.X {
			break
		}

		eighths := scaleToEighths(bar.Value, max, plot.Dy())
		drawVerticalBar(buf, plot, x, barWidth, eighths, ui.NewStyle(b.barColor(bar)))

		// The value goes above the bar and the label below, so neither is drawn
		// over the fill.
		if valueRow >= 0 {
			value := bar.Detail
			if value == "" {
				value = formatMetric(bar.Value, b.Unit)
			}
			putString(buf, inner, fitCell(value, barWidth, AlignRight),
				image.Pt(x, valueRow), textStyle)
		}

		putString(buf, inner, fitCell(bar.Label, barWidth, AlignLeft),
			image.Pt(x, labelRow), textStyle)

		x += slot
	}

	// Say so when bars were dropped, rather than silently showing a subset.
	if len(bars) < len(b.Bars) {
		note := "+" + itoaSmall(len(b.Bars)-len(bars))
		putString(buf, inner, note,
			image.Pt(inner.Max.X-len(note), labelRow), ui.NewStyle(ui.ColorYellow))
	}
}

// itoaSmall renders a small non-negative integer without importing strconv here.
func itoaSmall(n int) string {
	if n <= 0 {
		return "0"
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
