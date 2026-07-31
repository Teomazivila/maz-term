package ui

import (
	"fmt"
	"image"

	ui "github.com/gizak/termui/v3"
)

// Meter shows a percentage as a labelled horizontal bar.
//
// It replaces termui's Gauge, which fills its whole box with one solid colour: at
// the height a dashboard row gives it, that renders as a large block of red or
// blue carrying a single number, which is a poor use of the space. This draws a
// bar with eighth-width precision on one row and puts the reading and any detail
// on the same line, so a gauge costs one row instead of seven.
type Meter struct {
	*ui.Block

	Label   string
	Percent float64

	// Detail is shown after the reading, for example "18.1/24.0 GiB".
	Detail string

	// BarColor overrides the threshold grading when set.
	//
	// A pointer, because ui.Color's zero value is ColorBlack rather than
	// ColorClear: comparing against ColorClear to detect "unset" silently yielded
	// a black bar on a black background.
	BarColor *ui.Color

	// EmptyColor draws the unfilled remainder, so the bar's extent is visible.
	EmptyColor ui.Color

	TextColor ui.Color

	// LabelWidth fixes the label column so stacked meters align.
	LabelWidth int
}

// NewMeter creates a meter. An empty title draws no border, for use inline.
func NewMeter(title string) *Meter {
	block := ui.NewBlock()
	block.Title = title
	block.BorderStyle.Fg = ui.ColorCyan

	return &Meter{
		Block:      block,
		EmptyColor: ui.ColorClear,
		TextColor:  ui.ColorWhite,
		LabelWidth: 8,
	}
}

// barColor returns the colour for the current reading.
func (m *Meter) barColor() ui.Color {
	if m.BarColor != nil {
		return *m.BarColor
	}
	return thresholdColor(m.Percent)
}

// Draw renders the meter.
//
// With a single interior row everything shares that row. With more, the reading
// goes above a full-width bar, which reads better in a taller box.
func (m *Meter) Draw(buf *ui.Buffer) {
	m.Block.Draw(buf)

	inner := m.Inner
	if inner.Dx() < 4 || inner.Dy() < 1 {
		return
	}

	textStyle := ui.NewStyle(m.TextColor)
	reading := fmt.Sprintf("%5.1f%%", m.Percent)

	if inner.Dy() == 1 {
		m.drawSingleRow(buf, inner, reading, textStyle)
		return
	}

	m.drawStacked(buf, inner, reading, textStyle)
}

// drawSingleRow lays out label, bar, reading and detail on one line.
func (m *Meter) drawSingleRow(buf *ui.Buffer, inner image.Rectangle, reading string, textStyle ui.Style) {
	x := inner.Min.X

	if m.Label != "" {
		label := fitCell(m.Label, min(m.LabelWidth, inner.Dx()), AlignLeft)
		x += putString(buf, inner, label, image.Pt(x, inner.Min.Y), textStyle)
		x++
	}

	// The reading and detail are reserved first: a bar that has squeezed out its
	// own value is useless.
	trailing := " " + reading
	if m.Detail != "" {
		trailing += "  " + m.Detail
	}

	barWidth := inner.Max.X - x - len([]rune(trailing))
	if barWidth < 4 {
		// Not enough room for a bar; the numbers matter more.
		putString(buf, inner, fitCell(reading+" "+m.Detail, inner.Max.X-x, AlignLeft),
			image.Pt(x, inner.Min.Y), textStyle)
		return
	}

	m.drawBar(buf, image.Rect(x, inner.Min.Y, x+barWidth, inner.Min.Y+1), inner.Min.Y)
	putString(buf, inner, trailing, image.Pt(x+barWidth, inner.Min.Y), textStyle)
}

// drawStacked puts the reading above a full-width bar.
func (m *Meter) drawStacked(buf *ui.Buffer, inner image.Rectangle, reading string, textStyle ui.Style) {
	header := reading
	if m.Label != "" {
		header = m.Label + "  " + reading
	}
	if m.Detail != "" {
		header += "  " + m.Detail
	}
	putString(buf, inner, fitCell(header, inner.Dx(), AlignLeft),
		image.Pt(inner.Min.X, inner.Min.Y), textStyle)

	barRow := inner.Min.Y + 1
	m.drawBar(buf, image.Rect(inner.Min.X, barRow, inner.Max.X, barRow+1), barRow)
}

// drawBar fills the track then overlays the reading.
func (m *Meter) drawBar(buf *ui.Buffer, rect image.Rectangle, y int) {
	// The empty track makes the bar's full extent visible, so a low reading is
	// distinguishable from a chart that has not loaded.
	if m.EmptyColor != ui.ColorClear {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			buf.SetCell(ui.NewCell(horizontalBlocks[0], ui.NewStyle(m.EmptyColor)), image.Pt(x, y))
		}
	}

	drawHorizontalBar(buf, rect, y, m.Percent/100, ui.NewStyle(m.barColor()))
}
