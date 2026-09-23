package ui

import (
	"image"
	"strings"

	ui "github.com/gizak/termui/v3"
)

// Align controls horizontal cell alignment.
type Align int

const (
	// AlignLeft is the default for text.
	AlignLeft Align = iota
	// AlignRight suits numbers, so digits line up by magnitude.
	AlignRight
)

const (
	// columnGap separates columns. Two spaces reads better than a drawn rule and
	// costs less width.
	columnGap = 2

	// minColumnWidth is the narrowest a flexible column is allowed to become
	// before it is dropped entirely.
	minColumnWidth = 4

	// ellipsis marks a truncated cell.
	cellEllipsis = '…'
)

// Column describes one table column.
type Column struct {
	Title string

	// Width fixes the column's width. Zero makes it flexible, sharing the
	// leftover space in proportion to Weight.
	Width int

	// Weight is the share of leftover width a flexible column receives. Zero is
	// treated as one.
	Weight int

	Align Align
}

// Row is one table row. An unset Style falls back to the table's RowStyle.
type Row struct {
	Cells []string
	Style *ui.Style
}

// DataTable renders aligned rows under a header, without drawn column rules.
//
// termui's own Table is not adequate here: it takes literal column widths with no
// notion of a flexible column, so a width of zero silently renders the column as
// a single ellipsis one cell to the left of where it belongs. Every table in this
// dashboard lost its final column that way, which is why commit messages, process
// command lines and filenames were missing from the display.
//
// This widget computes widths from the space actually available, truncates with an
// ellipsis at the correct position, right-aligns numeric columns, and drops
// columns that cannot fit rather than corrupting the row.
type DataTable struct {
	// Embedded by pointer: ui.Block contains a mutex, so copying one by value
	// duplicates lock state, which go vet correctly refuses.
	*ui.Block

	Columns []Column
	Rows    []Row

	HeaderStyle   ui.Style
	RowStyle      ui.Style
	SelectedStyle ui.Style

	// SelectedRow is an index into Rows, or -1 for no selection.
	SelectedRow int

	// topRow is the first visible row, maintained so the selection stays on
	// screen as it moves.
	topRow int
}

// NewDataTable creates an empty table.
func NewDataTable(title string) *DataTable {
	block := ui.NewBlock()
	block.Title = title
	block.BorderStyle.Fg = ui.ColorCyan

	return &DataTable{
		Block:         block,
		HeaderStyle:   ui.NewStyle(ui.ColorWhite, ui.ColorClear, ui.ModifierBold),
		RowStyle:      ui.NewStyle(ui.ColorWhite),
		SelectedStyle: ui.NewStyle(ui.ColorBlack, ui.ColorCyan),
		SelectedRow:   -1,
	}
}

// SetRows replaces the table's rows, clamping the selection and scroll offset.
func (t *DataTable) SetRows(rows []Row) {
	t.Rows = rows

	if t.SelectedRow >= len(rows) {
		t.SelectedRow = len(rows) - 1
	}
	if t.topRow > len(rows) {
		t.topRow = 0
	}
}

// visibleRows is how many data rows fit below the header.
func (t *DataTable) visibleRows() int {
	height := t.Inner.Dy() - 1 // one line for the header
	if height < 0 {
		return 0
	}
	return height
}

// layout returns the drawn width of each column, and whether it is drawn at all.
//
// Fixed columns are honoured while they fit. The remainder is shared between
// flexible columns by weight. Columns that cannot reach minColumnWidth are
// dropped, so a narrow terminal loses trailing columns instead of rendering
// unreadable fragments of all of them.
func (t *DataTable) layout(width int) []int {
	widths := make([]int, len(t.Columns))
	if len(t.Columns) == 0 || width <= 0 {
		return widths
	}

	available := width - columnGap*(len(t.Columns)-1)
	if available < 0 {
		available = 0
	}

	// Fixed columns first, in order, while budget remains.
	remaining := available
	totalWeight := 0
	for i, column := range t.Columns {
		switch {
		case column.Width > 0:
			take := min(column.Width, remaining)
			widths[i] = take
			remaining -= take
		default:
			weight := max(column.Weight, 1)
			totalWeight += weight
		}
	}

	// Then share what is left among the flexible columns.
	if totalWeight > 0 && remaining > 0 {
		granted := 0
		lastFlexible := -1
		for i, column := range t.Columns {
			if column.Width > 0 {
				continue
			}
			lastFlexible = i
			share := remaining * max(column.Weight, 1) / totalWeight
			widths[i] = share
			granted += share
		}
		// The final flexible column absorbs the rounding remainder so the row
		// reaches the right edge exactly.
		if lastFlexible >= 0 {
			widths[lastFlexible] += remaining - granted
		}
	}

	// Drop anything too narrow to be legible.
	for i := range widths {
		if widths[i] < minColumnWidth {
			widths[i] = 0
		}
	}

	return widths
}

// Draw renders the table.
func (t *DataTable) Draw(buf *ui.Buffer) {
	t.Block.Draw(buf)

	if t.Inner.Dy() < 1 || t.Inner.Dx() < 1 {
		return
	}

	widths := t.layout(t.Inner.Dx())

	// Header.
	t.drawRow(buf, t.Inner.Min.Y, widths, t.headerCells(), t.HeaderStyle, false)

	visible := t.visibleRows()
	if visible == 0 {
		return
	}

	t.scrollToSelection(visible)

	for offset := 0; offset < visible; offset++ {
		index := t.topRow + offset
		if index >= len(t.Rows) {
			break
		}

		row := t.Rows[index]

		style := t.RowStyle
		if row.Style != nil {
			style = *row.Style
		}

		selected := index == t.SelectedRow
		if selected {
			style = t.SelectedStyle
		}

		t.drawRow(buf, t.Inner.Min.Y+1+offset, widths, row.Cells, style, selected)
	}
}

// headerCells returns the column titles.
func (t *DataTable) headerCells() []string {
	cells := make([]string, len(t.Columns))
	for i, column := range t.Columns {
		cells[i] = column.Title
	}
	return cells
}

// scrollToSelection adjusts the scroll offset so the selection stays visible.
func (t *DataTable) scrollToSelection(visible int) {
	if t.SelectedRow < 0 {
		if t.topRow > 0 && t.topRow >= len(t.Rows) {
			t.topRow = 0
		}
		return
	}

	if t.SelectedRow < t.topRow {
		t.topRow = t.SelectedRow
	}
	if t.SelectedRow >= t.topRow+visible {
		t.topRow = t.SelectedRow - visible + 1
	}
	if t.topRow < 0 {
		t.topRow = 0
	}
}

// drawRow writes one row of cells at y.
func (t *DataTable) drawRow(buf *ui.Buffer, y int, widths []int, cells []string, style ui.Style, fill bool) {
	if y >= t.Inner.Max.Y {
		return
	}

	// A selected row is filled across its whole width so the highlight reads as
	// one bar rather than per-cell blocks.
	if fill {
		buf.Fill(ui.NewCell(' ', style),
			image.Rect(t.Inner.Min.X, y, t.Inner.Max.X, y+1))
	}

	x := t.Inner.Min.X
	for i, width := range widths {
		if width == 0 {
			continue
		}
		if x >= t.Inner.Max.X {
			break
		}

		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}

		text := fitCell(cell, width, t.Columns[i].Align)

		for offset, r := range []rune(text) {
			if x+offset >= t.Inner.Max.X {
				break
			}
			buf.SetCell(ui.NewCell(r, style), image.Pt(x+offset, y))
		}

		x += width + columnGap
	}
}

// fitCell truncates or pads text to exactly width runes.
func fitCell(text string, width int, align Align) string {
	if width <= 0 {
		return ""
	}

	// Control characters would corrupt the frame, and a value from a remote API
	// or a filename cannot be assumed free of them.
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)

	runes := []rune(text)

	switch {
	case len(runes) > width:
		if width == 1 {
			return string(cellEllipsis)
		}
		return string(runes[:width-1]) + string(cellEllipsis)
	case len(runes) == width:
		return text
	}

	padding := strings.Repeat(" ", width-len(runes))
	if align == AlignRight {
		return padding + text
	}
	return text + padding
}

// MoveSelection shifts the selection by delta, clamped to the row range.
func (t *DataTable) MoveSelection(delta int) {
	if len(t.Rows) == 0 {
		t.SelectedRow = -1
		return
	}

	next := t.SelectedRow + delta
	if t.SelectedRow < 0 && delta > 0 {
		next = 0
	}

	t.SelectedRow = min(max(next, 0), len(t.Rows)-1)
}
