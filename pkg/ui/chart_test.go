package ui

import (
	"image"
	"strings"
	"testing"

	ui "github.com/gizak/termui/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drawWidget renders any drawable and returns its lines.
func drawWidget(t *testing.T, d ui.Drawable, width, height int) []string {
	t.Helper()

	rect := image.Rect(0, 0, width, height)
	d.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	buf := ui.NewBuffer(rect)
	d.Draw(buf)

	lines := make([]string, 0, height)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		var b strings.Builder
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r := buf.GetCell(image.Pt(x, y)).Rune
			if r == 0 {
				r = ' '
			}
			b.WriteRune(r)
		}
		lines = append(lines, b.String())
	}
	return lines
}

// TestSubCellGivesEightfoldResolution pins the technique that makes these charts
// read as curves rather than stacks of solid rectangles: whole cells plus one
// eighth-height rune for the remainder.
func TestSubCellGivesEightfoldResolution(t *testing.T) {
	tests := []struct {
		eighths     int
		wantFull    int
		wantPartial rune
	}{
		{eighths: 0, wantFull: 0, wantPartial: 0},
		{eighths: 1, wantFull: 0, wantPartial: '▁'},
		{eighths: 4, wantFull: 0, wantPartial: '▄'},
		{eighths: 7, wantFull: 0, wantPartial: '▇'},
		{eighths: 8, wantFull: 1, wantPartial: 0},
		{eighths: 9, wantFull: 1, wantPartial: '▁'},
		{eighths: 20, wantFull: 2, wantPartial: '▄'},
		{eighths: 24, wantFull: 3, wantPartial: 0},
	}

	for _, tt := range tests {
		full, partial := subCell(tt.eighths)
		assert.Equal(t, tt.wantFull, full, "eighths=%d", tt.eighths)
		assert.Equal(t, tt.wantPartial, partial, "eighths=%d", tt.eighths)
	}

	// Negative input must not produce a bar.
	full, partial := subCell(-5)
	assert.Zero(t, full)
	assert.Zero(t, partial)
}

func TestScaleToEighths(t *testing.T) {
	// A full-scale value fills every eighth of every row.
	assert.Equal(t, 6*8, scaleToEighths(100, 100, 6))

	// Half scale fills half.
	assert.Equal(t, 24, scaleToEighths(50, 100, 6))

	// Overshoot is clamped rather than drawn outside the plot.
	assert.Equal(t, 6*8, scaleToEighths(250, 100, 6))

	// Degenerate inputs produce nothing.
	assert.Zero(t, scaleToEighths(50, 0, 6))
	assert.Zero(t, scaleToEighths(50, 100, 0))
	assert.Zero(t, scaleToEighths(-5, 100, 6))
}

func TestNiceMax(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{in: 0, want: 1},
		{in: -3, want: 1},
		{in: 0.8, want: 1},
		{in: 1, want: 1},
		{in: 1.5, want: 2},
		{in: 3.2, want: 5},
		{in: 7, want: 10},
		{in: 33.4, want: 50},
		{in: 61, want: 100},
		{in: 4707, want: 5000},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, niceMax(tt.in), "niceMax(%v)", tt.in)
	}
}

func TestThresholdColor(t *testing.T) {
	assert.Equal(t, ui.ColorGreen, thresholdColor(10))
	assert.Equal(t, ui.ColorGreen, thresholdColor(74.9))
	assert.Equal(t, ui.ColorYellow, thresholdColor(75))
	assert.Equal(t, ui.ColorYellow, thresholdColor(89.9))
	assert.Equal(t, ui.ColorRed, thresholdColor(90))
	assert.Equal(t, ui.ColorRed, thresholdColor(150))
}

func TestStatsOf(t *testing.T) {
	stats := statsOf([]float64{4, 10, 6})

	assert.Equal(t, 6.0, stats.Current, "current is the newest sample, not the largest")
	assert.Equal(t, 4.0, stats.Min)
	assert.Equal(t, 10.0, stats.Max)
	assert.InDelta(t, 6.667, stats.Mean, 0.01)
	assert.Equal(t, 3, stats.Count)

	assert.Zero(t, statsOf(nil).Count)
}

// TestPutStringNeutralisesControlCharacters keeps a hostile label from corrupting
// the frame.
func TestPutStringNeutralisesControlCharacters(t *testing.T) {
	rect := image.Rect(0, 0, 12, 2)
	buf := ui.NewBuffer(rect)

	putString(buf, rect, "a\x1bb\nc", image.Pt(0, 0), ui.NewStyle(ui.ColorWhite))

	for x := range 5 {
		r := buf.GetCell(image.Pt(x, 0)).Rune
		assert.NotEqual(t, '\x1b', r)
		assert.NotEqual(t, '\n', r)
	}
}

func TestPutStringClipsToRect(t *testing.T) {
	rect := image.Rect(0, 0, 5, 2)
	buf := ui.NewBuffer(rect)

	used := putString(buf, rect, "abcdefghij", image.Pt(0, 0), ui.NewStyle(ui.ColorWhite))
	assert.Equal(t, 5, used, "writing must stop at the right edge")

	// Writing outside the rect writes nothing.
	assert.Zero(t, putString(buf, rect, "x", image.Pt(0, 99), ui.NewStyle(ui.ColorWhite)))
}

// --- Sparkline ---------------------------------------------------------------

// TestSparklineUsesFullHeight is the difference from termui's own sparkline, which
// draws a single row and so compresses any series into eight levels.
func TestSparklineUsesFullHeight(t *testing.T) {
	spark := NewSparkline("CPU")
	spark.Max = 100
	spark.ShowAxis = false
	spark.ShowLegend = false
	spark.Data = []float64{100, 100, 100}

	lines := drawWidget(t, spark, 20, 8)

	// A full-scale series must reach the top of the plot, not just the bottom row.
	rowsWithBlocks := 0
	for _, line := range lines[1 : len(lines)-1] {
		if strings.ContainsRune(line, fullBlock) {
			rowsWithBlocks++
		}
	}
	assert.GreaterOrEqual(t, rowsWithBlocks, 5,
		"a full-scale series must fill the plot's height, not one row")
}

func TestSparklineNewestSampleIsRightmost(t *testing.T) {
	spark := NewSparkline("S")
	spark.Max = 100
	spark.ShowAxis = false
	spark.ShowLegend = false
	// Only the last sample is non-zero.
	spark.Data = []float64{0, 0, 0, 100}

	lines := drawWidget(t, spark, 12, 6)

	// Compared in runes, not bytes: the block runes are three bytes each in UTF-8,
	// so a byte offset is not a column.
	interior := []rune(lines[2])
	lastBlock := -1
	for i, r := range interior {
		if r == fullBlock || strings.ContainsRune(string(verticalBlocks[:]), r) {
			lastBlock = i
		}
	}
	assert.Equal(t, len(interior)-2, lastBlock,
		"the newest sample must be drawn against the right edge")
}

func TestSparklineAutoScalesWhenMaxUnset(t *testing.T) {
	spark := NewSparkline("S")
	spark.Data = []float64{10, 20, 33.4}

	// niceMax rounds 33.4 up to 50, so the axis reads as a scale.
	assert.Equal(t, 50.0, spark.scaleMax())

	spark.Max = 100
	assert.Equal(t, 100.0, spark.scaleMax(), "an explicit max wins")

	empty := NewSparkline("S")
	assert.Equal(t, 1.0, empty.scaleMax(), "an empty series must not divide by zero")
}

func TestSparklineLegendReportsStats(t *testing.T) {
	spark := NewSparkline("S")
	spark.Unit = "%"
	spark.Data = []float64{4, 10, 6}

	lines := drawWidget(t, spark, 60, 8)
	body := strings.Join(lines, "\n")

	assert.Contains(t, body, "now 6", "the legend reports the newest value")
	assert.Contains(t, body, "peak 10")
	assert.Contains(t, body, "n=3")
}

func TestSparklineEmptySeriesSaysSo(t *testing.T) {
	spark := NewSparkline("S")

	lines := drawWidget(t, spark, 40, 8)
	assert.Contains(t, strings.Join(lines, "\n"), "collecting")
}

func TestSparklineAppendCapsSeries(t *testing.T) {
	spark := NewSparkline("S")
	for i := range 50 {
		spark.Append(float64(i), 10)
	}

	require.Len(t, spark.Data, 10, "the series must be bounded")
	assert.Equal(t, 49.0, spark.Data[len(spark.Data)-1], "the newest sample is kept")
	assert.Equal(t, 40.0, spark.Data[0], "the oldest are discarded")
}

func TestSparklineSurvivesTinyRects(t *testing.T) {
	spark := NewSparkline("S")
	spark.Data = []float64{1, 2, 3}

	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {40, 2}, {2, 40}} {
		assert.NotPanics(t, func() { drawWidget(t, spark, size[0], size[1]) },
			"drawing at %dx%d panicked", size[0], size[1])
	}
}

// --- Meter -------------------------------------------------------------------

// TestMeterFitsOneInteriorRow is the point of replacing termui's Gauge, which
// filled its whole box with one colour.
func TestMeterFitsOneInteriorRow(t *testing.T) {
	meter := NewMeter("CPU")
	meter.Label = "cpu"
	meter.Percent = 42.5
	meter.Detail = "load 1.2"

	lines := drawWidget(t, meter, 60, 3)

	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], "cpu", "the label is on the single interior row")
	assert.Contains(t, lines[1], "42.5%", "so is the reading")
	assert.Contains(t, lines[1], "load 1.2", "and the detail")
	assert.Contains(t, lines[1], string(fullBlock), "and the bar")
}

func TestMeterBarLengthTracksPercent(t *testing.T) {
	count := func(percent float64) int {
		meter := NewMeter("")
		meter.Label = ""
		meter.Percent = percent
		meter.EmptyColor = ui.ColorClear

		lines := drawWidget(t, meter, 42, 3)
		return strings.Count(lines[1], string(fullBlock))
	}

	assert.Zero(t, count(0))
	quarter, half, full := count(25), count(50), count(100)

	assert.Greater(t, half, quarter, "a higher reading draws a longer bar")
	assert.Greater(t, full, half)
}

func TestMeterGradesByThreshold(t *testing.T) {
	meter := NewMeter("")

	meter.Percent = 20
	assert.Equal(t, ui.ColorGreen, meter.barColor())

	meter.Percent = 80
	assert.Equal(t, ui.ColorYellow, meter.barColor())

	meter.Percent = 95
	assert.Equal(t, ui.ColorRed, meter.barColor())

	blue := ui.ColorBlue
	meter.BarColor = &blue
	assert.Equal(t, ui.ColorBlue, meter.barColor(), "an explicit colour wins")
}

// TestMeterKeepsNumbersWhenTooNarrowForABar prefers the reading to a bar stub.
func TestMeterKeepsNumbersWhenTooNarrowForABar(t *testing.T) {
	meter := NewMeter("")
	meter.Label = "memory"
	meter.Percent = 61.5

	lines := drawWidget(t, meter, 20, 3)
	assert.Contains(t, lines[1], "61.5%")
}

func TestMeterStacksWhenGivenHeight(t *testing.T) {
	meter := NewMeter("CPU")
	meter.Label = "cpu"
	meter.Percent = 50

	lines := drawWidget(t, meter, 40, 4)

	assert.Contains(t, lines[1], "50.0%", "the reading goes on the first row")
	assert.Contains(t, lines[2], string(fullBlock), "the bar gets its own row")
}

func TestMeterSurvivesTinyRects(t *testing.T) {
	meter := NewMeter("m")
	meter.Percent = 50

	for _, size := range [][2]int{{0, 0}, {1, 1}, {3, 3}, {5, 2}} {
		assert.NotPanics(t, func() { drawWidget(t, meter, size[0], size[1]) },
			"drawing at %dx%d panicked", size[0], size[1])
	}
}

func TestMeterClampsOutOfRangePercent(t *testing.T) {
	for _, percent := range []float64{-50, 0, 100, 250} {
		meter := NewMeter("")
		meter.Percent = percent
		assert.NotPanics(t, func() { drawWidget(t, meter, 30, 3) },
			"percent=%v panicked", percent)
	}
}

// --- BarChart ----------------------------------------------------------------

// TestBarChartSeparatesValuesAndLabels pins the fix for termui's chart, which drew
// numbers inside the bars where they collided with the fill.
func TestBarChartSeparatesValuesAndLabels(t *testing.T) {
	chart := NewBarChart("Disk")
	chart.Max = 100
	chart.Bars = []Bar{
		{Label: "root", Value: 30, Detail: "30%"},
		{Label: "data", Value: 90, Detail: "90%"},
	}

	lines := drawWidget(t, chart, 40, 9)

	valueRow, labelRow := lines[1], lines[len(lines)-2]

	assert.Contains(t, valueRow, "30%", "values go above the bars")
	assert.Contains(t, valueRow, "90%")
	assert.NotContains(t, valueRow, string(fullBlock), "and not over the fill")

	assert.Contains(t, labelRow, "root", "labels go below the bars")
	assert.Contains(t, labelRow, "data")
}

func TestBarChartHeightTracksValue(t *testing.T) {
	chart := NewBarChart("C")
	chart.Max = 100
	chart.Bars = []Bar{{Label: "lo", Value: 10}, {Label: "hi", Value: 100}}

	lines := drawWidget(t, chart, 30, 10)

	// The tall bar occupies more rows than the short one.
	var loRows, hiRows int
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) < 20 {
			continue
		}
		if runes[2] == fullBlock || strings.ContainsRune(string(verticalBlocks[:]), runes[2]) {
			loRows++
		}
		if runes[17] == fullBlock || strings.ContainsRune(string(verticalBlocks[:]), runes[17]) {
			hiRows++
		}
	}
	assert.Greater(t, hiRows, loRows, "a larger value must draw a taller bar")
}

// TestBarChartDropsBarsItCannotDrawLegibly prefers fewer readable bars to many
// unreadable slivers, and says how many it dropped.
func TestBarChartDropsBarsItCannotDrawLegibly(t *testing.T) {
	chart := NewBarChart("C")
	chart.Max = 100
	for i := range 20 {
		chart.Bars = append(chart.Bars, Bar{Label: "m" + itoaSmall(i), Value: float64(i * 5)})
	}

	lines := drawWidget(t, chart, 26, 8)
	labelRow := lines[len(lines)-2]

	assert.Contains(t, labelRow, "+", "the number of dropped bars must be shown")
}

func TestBarChartAutoScales(t *testing.T) {
	chart := NewBarChart("C")
	chart.Bars = []Bar{{Value: 12}, {Value: 33.4}}

	assert.Equal(t, 50.0, chart.scaleMax())

	chart.Max = 100
	assert.Equal(t, 100.0, chart.scaleMax())

	empty := NewBarChart("C")
	assert.Equal(t, 1.0, empty.scaleMax())
}

func TestBarChartSurvivesTinyRectsAndNoData(t *testing.T) {
	chart := NewBarChart("C")

	assert.NotPanics(t, func() { drawWidget(t, chart, 30, 8) }, "empty chart panicked")

	chart.Bars = []Bar{{Label: "a", Value: 1}}
	for _, size := range [][2]int{{0, 0}, {1, 1}, {3, 3}, {4, 2}} {
		assert.NotPanics(t, func() { drawWidget(t, chart, size[0], size[1]) },
			"drawing at %dx%d panicked", size[0], size[1])
	}
}

func TestItoaSmall(t *testing.T) {
	assert.Equal(t, "0", itoaSmall(0))
	assert.Equal(t, "0", itoaSmall(-4))
	assert.Equal(t, "7", itoaSmall(7))
	assert.Equal(t, "123", itoaSmall(123))
}

func TestFormatMetric(t *testing.T) {
	assert.Equal(t, "0%", formatMetric(0, "%"))
	assert.Equal(t, "12.5%", formatMetric(12.5, "%"))
	assert.Equal(t, "150.2ms", formatMetric(150.23, "ms"))
	assert.Equal(t, "4708ms", formatMetric(4707.8, "ms"))
}
