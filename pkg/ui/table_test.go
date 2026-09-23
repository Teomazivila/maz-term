package ui

import (
	"image"
	"strings"
	"testing"

	ui "github.com/gizak/termui/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drawTable renders a table into a buffer and returns its visible lines.
func drawTable(t *testing.T, table *DataTable, width, height int) []string {
	t.Helper()

	rect := image.Rect(0, 0, width, height)
	table.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

	buf := ui.NewBuffer(rect)
	table.Draw(buf)

	lines := make([]string, 0, height)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		var b strings.Builder
		for x := rect.Min.X; x < rect.Max.X; x++ {
			b.WriteRune(buf.GetCell(image.Pt(x, y)).Rune)
		}
		lines = append(lines, strings.TrimRight(b.String(), " \x00"))
	}
	return lines
}

// TestFlexibleColumnIsVisible is the regression test for the defect that made the
// dashboard unreadable.
//
// termui's Table takes literal widths, so the zero used to mean "fill the rest"
// rendered the column as a single ellipsis one cell to the left of where it
// belonged. Every table lost its final column that way: commit messages, process
// command lines, filenames and node names were all absent from the display.
func TestFlexibleColumnIsVisible(t *testing.T) {
	table := newTable("Commits",
		Column{Title: "COMMIT", Width: 9},
		Column{Title: "AUTHOR", Width: 10},
		Column{Title: "MESSAGE", Weight: 1})
	table.SetRows([]Row{
		textRow("abc12345", "Alice", "fix: make the last column visible"),
	})

	lines := drawTable(t, table, 70, 6)
	body := strings.Join(lines, "\n")

	assert.Contains(t, body, "MESSAGE", "the flexible column's header must appear")
	assert.Contains(t, body, "fix: make the last column visible",
		"the flexible column's content must appear")
	assert.Contains(t, body, "abc12345")
	assert.Contains(t, body, "Alice")
}

// TestNoColumnSeparators pins the absence of the drawn rules that made rows read
// as noise.
func TestNoColumnSeparators(t *testing.T) {
	table := newTable("T",
		Column{Title: "A", Width: 6},
		Column{Title: "B", Weight: 1})
	table.SetRows([]Row{textRow("one", "two")})

	lines := drawTable(t, table, 40, 5)

	// Row 1 is the header, row 2 the data; neither may contain a vertical rule
	// inside the frame.
	for _, line := range lines[1:3] {
		interior := strings.Trim(line, "│| ")
		assert.NotContains(t, interior, "|", "columns must be separated by spacing, not rules")
	}
}

func TestColumnAlignment(t *testing.T) {
	table := newTable("T",
		Column{Title: "NAME", Width: 8},
		Column{Title: "COUNT", Width: 6, Align: AlignRight})
	table.SetRows([]Row{textRow("api", "7"), textRow("worker", "1234")})

	lines := drawTable(t, table, 30, 6)

	// Right-aligned numbers end at the same column, which is the point of the
	// alignment: magnitudes line up.
	first := strings.Index(lines[2], "7")
	second := strings.Index(lines[3], "1234") + len("1234") - 1
	assert.Equal(t, first, second, "right-aligned cells must share a right edge")
}

func TestCellTruncationUsesEllipsis(t *testing.T) {
	table := newTable("T", Column{Title: "PATH", Width: 10})
	table.SetRows([]Row{textRow("/very/long/path/that/cannot/fit")})

	lines := drawTable(t, table, 20, 4)

	assert.Contains(t, lines[2], "…", "an overlong cell must be marked as truncated")
	assert.NotContains(t, lines[2], "cannot/fit")
}

func TestFitCell(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		align Align
		want  string
	}{
		{name: "pads left-aligned", text: "ab", width: 5, want: "ab   "},
		{name: "pads right-aligned", text: "ab", width: 5, align: AlignRight, want: "   ab"},
		{name: "exact fit", text: "abcde", width: 5, want: "abcde"},
		{name: "truncates", text: "abcdefg", width: 5, want: "abcd…"},
		{name: "width one truncates to ellipsis", text: "abc", width: 1, want: "…"},
		{name: "zero width is empty", text: "abc", width: 0, want: ""},
		{name: "negative width is empty", text: "abc", width: -3, want: ""},
		{name: "empty text pads", text: "", width: 3, want: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fitCell(tt.text, tt.width, tt.align))
		})
	}
}

// TestFitCellNeutralisesControlCharacters keeps a hostile value from a remote API
// or an odd filename from corrupting the frame.
func TestFitCellNeutralisesControlCharacters(t *testing.T) {
	got := fitCell("a\x1b[31mb\nc", 10, AlignLeft)

	assert.NotContains(t, got, "\x1b")
	assert.NotContains(t, got, "\n")
	assert.Len(t, []rune(got), 10)
}

// TestFitCellHandlesMultiByteRunes checks truncation counts runes, not bytes, so a
// cut cannot produce invalid UTF-8.
func TestFitCellHandlesMultiByteRunes(t *testing.T) {
	got := fitCell("héllo wörld", 6, AlignLeft)

	assert.Len(t, []rune(got), 6)
	assert.Equal(t, "héllo…", got)
}

func TestLayoutSharesLeftoverWidthByWeight(t *testing.T) {
	table := newTable("T",
		Column{Title: "FIXED", Width: 10},
		Column{Title: "ONE", Weight: 1},
		Column{Title: "THREE", Weight: 3})

	// 10 fixed + 2 gaps of 2 = 14 consumed; 46 shared 1:3.
	widths := table.layout(60)

	require.Len(t, widths, 3)
	assert.Equal(t, 10, widths[0])
	assert.Equal(t, 46, widths[1]+widths[2], "flexible columns must consume the remainder")
	assert.Greater(t, widths[2], widths[1], "a heavier weight gets more width")
}

// TestLayoutDropsColumnsThatCannotFit prefers losing trailing columns to rendering
// unreadable fragments of all of them.
func TestLayoutDropsColumnsThatCannotFit(t *testing.T) {
	table := newTable("T",
		Column{Title: "A", Width: 20},
		Column{Title: "B", Width: 20},
		Column{Title: "C", Width: 20})

	widths := table.layout(25)

	assert.Equal(t, 20, widths[0], "the first column still fits")
	assert.Zero(t, widths[2], "a column with no room is dropped rather than mangled")
}

func TestLayoutHandlesDegenerateWidth(t *testing.T) {
	table := newTable("T", Column{Title: "A", Weight: 1})

	assert.Equal(t, []int{0}, table.layout(0))
	assert.Equal(t, []int{0}, table.layout(-5))
}

func TestSelectionMovesAndClamps(t *testing.T) {
	table := newTable("T", Column{Title: "A", Weight: 1})
	table.SetRows([]Row{textRow("a"), textRow("b"), textRow("c")})

	assert.Equal(t, -1, table.SelectedRow, "nothing is selected initially")

	table.MoveSelection(1)
	assert.Equal(t, 0, table.SelectedRow, "moving down from nothing selects the first row")

	table.MoveSelection(1)
	table.MoveSelection(1)
	table.MoveSelection(1)
	assert.Equal(t, 2, table.SelectedRow, "selection clamps at the last row")

	table.MoveSelection(-10)
	assert.Equal(t, 0, table.SelectedRow, "selection clamps at the first row")
}

func TestSelectionOnEmptyTable(t *testing.T) {
	table := newTable("T", Column{Title: "A", Weight: 1})

	table.MoveSelection(1)
	assert.Equal(t, -1, table.SelectedRow)
}

// TestSetRowsClampsSelection stops a shrinking list leaving the selection past the
// end, which would index out of range while drawing.
func TestSetRowsClampsSelection(t *testing.T) {
	table := newTable("T", Column{Title: "A", Weight: 1})
	table.SetRows([]Row{textRow("a"), textRow("b"), textRow("c")})
	table.SelectedRow = 2

	table.SetRows([]Row{textRow("a")})
	assert.Equal(t, 0, table.SelectedRow)

	table.SetRows(nil)
	assert.Equal(t, -1, table.SelectedRow)
}

// TestScrollFollowsSelection is what makes a 487-row pod list usable.
func TestScrollFollowsSelection(t *testing.T) {
	table := newTable("Pods", Column{Title: "POD", Weight: 1})

	rows := make([]Row, 100)
	for i := range rows {
		rows[i] = textRow(padName(i))
	}
	table.SetRows(rows)

	// A short table: header plus a handful of rows.
	table.SetRect(0, 0, 30, 8)
	table.SelectedRow = 90

	lines := drawTable(t, table, 30, 8)
	body := strings.Join(lines, "\n")

	assert.Contains(t, body, padName(90), "the selected row must be scrolled into view")
	assert.NotContains(t, body, padName(0), "distant rows must scroll out of view")
}

func TestDrawWithNoRows(t *testing.T) {
	table := newTable("Empty", Column{Title: "A", Weight: 1})

	assert.NotPanics(t, func() { drawTable(t, table, 20, 5) })
}

func TestDrawInDegenerateRect(t *testing.T) {
	table := newTable("T", Column{Title: "A", Weight: 1})
	table.SetRows([]Row{textRow("x")})

	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {40, 2}} {
		assert.NotPanics(t, func() { drawTable(t, table, size[0], size[1]) },
			"drawing at %dx%d panicked", size[0], size[1])
	}
}

// TestRowWithFewerCellsThanColumns tolerates a caller that supplies a short row.
func TestRowWithFewerCellsThanColumns(t *testing.T) {
	table := newTable("T",
		Column{Title: "A", Width: 6},
		Column{Title: "B", Width: 6},
		Column{Title: "C", Weight: 1})
	table.SetRows([]Row{textRow("only-one")})

	assert.NotPanics(t, func() {
		lines := drawTable(t, table, 40, 5)
		assert.Contains(t, strings.Join(lines, "\n"), "only-")
	})
}

func TestNoticeRowFillsColumnCount(t *testing.T) {
	rows := noticeRow(4, "nothing here")

	require.Len(t, rows, 1)
	require.Len(t, rows[0].Cells, 4)
	assert.Equal(t, "nothing here", rows[0].Cells[0])
	assert.Empty(t, rows[0].Cells[3])
}

// padName gives rows distinguishable, searchable content.
func padName(i int) string {
	return "pod-" + strings.Repeat("0", 3-len(itoa(i))) + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// TestNothingIsDrawnOutsideTheFrame guards the border: writing even one cell past
// Inner.Max.X corrupts the frame and bleeds into the neighbouring widget.
func TestNothingIsDrawnOutsideTheFrame(t *testing.T) {
	table := newTable("T",
		Column{Title: "A", Width: 9},
		Column{Title: "B", Width: 12},
		Column{Title: "C", Weight: 1},
		Column{Title: "D", Weight: 2})
	table.SetRows([]Row{
		textRow(strings.Repeat("x", 40), strings.Repeat("y", 40),
			strings.Repeat("z", 80), strings.Repeat("w", 80)),
	})
	table.SelectedRow = 0

	for _, width := range []int{20, 31, 40, 57, 80, 121} {
		rect := image.Rect(0, 0, width, 6)
		table.SetRect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y)

		buf := ui.NewBuffer(rect)
		table.Draw(buf)

		// The right border column must still hold the border rune, not content.
		for y := table.Inner.Min.Y; y < table.Inner.Max.Y; y++ {
			border := buf.GetCell(image.Pt(rect.Max.X-1, y)).Rune
			assert.Equal(t, ui.VERTICAL_LINE, border,
				"content overwrote the right border at width %d, row %d", width, y)
		}
	}
}

// TestHeaderAndRowsShareColumnPositions is what makes the table scannable: a value
// must sit directly under its own heading.
func TestHeaderAndRowsShareColumnPositions(t *testing.T) {
	table := newTable("T",
		Column{Title: "AAA", Width: 6},
		Column{Title: "BBB", Width: 8},
		Column{Title: "CCC", Weight: 1})
	table.SetRows([]Row{textRow("a1", "b1", "c1")})

	lines := drawTable(t, table, 40, 5)
	header, row := lines[1], lines[2]

	for _, pair := range [][2]string{{"AAA", "a1"}, {"BBB", "b1"}, {"CCC", "c1"}} {
		assert.Equal(t, strings.Index(header, pair[0]), strings.Index(row, pair[1]),
			"%s and %s must start at the same column", pair[0], pair[1])
	}
}
