//go:build windows

package winui

import "math"

// Column is one column of a table.
type Column struct {
	Title string
	Width int // in the units of the layout; the last column takes what is left
}

// table is what a Control of the table kind holds besides the rest.
type table struct {
	columns   []Column
	rows      [][]string
	tints     []Colour // the colour of the first cell of each row; zero for the usual one
	picked    int      // the chosen row, or -1
	top       int      // the first row on show
	scrolling bool     // the pointer is holding the scroll bar
	onPick    func(row int)
}

// Table is a list of rows under a line of column titles, one row chosen at
// a time. It scrolls with the mouse wheel, the bar at its right and the
// arrow keys. onPick is called with the row the user chooses.
func (w *Window) Table(x, y, width, height int, columns []Column, onPick func(row int)) *Control {
	c := w.add(kindTable, x, y, width, height, "", nil)
	c.table = &table{columns: columns, picked: -1, onPick: onPick}
	return c
}

// Fill replaces every row of a table. A tint, when given, colours the
// first cell of its row.
func (c *Control) Fill(rows [][]string, tints []Colour) {
	t := c.table
	t.rows, t.tints, t.picked, t.top = rows, tints, -1, 0
	c.redraw()
}

// SetCell changes the text of one cell of a table.
func (c *Control) SetCell(row, column int, text string) {
	t := c.table
	if row < 0 || row >= len(t.rows) || column < 0 {
		return
	}
	for len(t.rows[row]) <= column {
		t.rows[row] = append(t.rows[row], "")
	}
	if t.rows[row][column] != text {
		t.rows[row][column] = text
		c.redraw()
	}
}

// The measures of a table, in pixels: the height of the line of titles and
// of a row, how many rows fit, and the width of the scroll bar.
func (c *Control) tableMeasures() (head, row, fit, bar int) {
	_, _, _, height := c.box()
	head, row, bar = c.w.px(32), c.w.px(26), c.w.px(14)
	return head, row, max((height-head-c.w.px(4))/row, 1), bar
}

// scrollTo shows the rows from the given one, as far as there are rows.
func (c *Control) scrollTo(top int) {
	_, _, fit, _ := c.tableMeasures()
	top = max(0, min(top, len(c.table.rows)-fit))
	if top != c.table.top {
		c.table.top = top
		c.redraw()
	}
}

// pick chooses a row, brings it into view and tells the program.
func (c *Control) pick(row int) {
	t := c.table
	if row < 0 || row >= len(t.rows) {
		return
	}
	_, _, fit, _ := c.tableMeasures()
	if row < t.top {
		c.scrollTo(row)
	} else if row >= t.top+fit {
		c.scrollTo(row - fit + 1)
	}
	if row != t.picked {
		t.picked = row
		c.redraw()
		if t.onPick != nil {
			t.onPick(row)
		}
	}
}

// thumb returns where the handle of the scroll bar is, from the top of its
// track, and how long it and the track are. length is zero when every row
// fits and there is no bar.
func (c *Control) thumb() (offset, length, track int) {
	_, _, _, height := c.box()
	head, _, fit, _ := c.tableMeasures()
	rows := len(c.table.rows)
	if rows <= fit {
		return 0, 0, 0
	}
	track = height - head - c.w.px(8)
	length = max(track*fit/rows, c.w.px(24))
	offset = (track - length) * c.table.top / (rows - fit)
	return offset, length, track
}

// tableDown handles a press of the mouse button on a table.
func (c *Control) tableDown(px, py int) {
	x, y, width, _ := c.box()
	head, row, fit, bar := c.tableMeasures()
	t := c.table
	if _, length, _ := c.thumb(); length > 0 && px >= x+width-bar {
		t.scrolling = true
		c.tableDrag(py)
		return
	}
	if py < y+head {
		return
	}
	if picked := t.top + (py-y-head)/row; picked < t.top+fit {
		c.pick(picked)
	}
}

// tableDrag moves the rows so the handle of the scroll bar is under the pointer.
func (c *Control) tableDrag(py int) {
	_, y, _, _ := c.box()
	head, _, fit, _ := c.tableMeasures()
	_, length, track := c.thumb()
	if track <= length {
		return
	}
	at := float64(py-y-head-c.w.px(4)-length/2) / float64(track-length)
	c.scrollTo(int(math.Round(at * float64(len(c.table.rows)-fit))))
}

// tableKey moves the chosen row with the keyboard and reports whether the
// key was one it uses.
func (c *Control) tableKey(key uint32) bool {
	t := c.table
	_, _, fit, _ := c.tableMeasures()
	row := t.picked
	switch key {
	case vkUp:
		row--
	case vkDown:
		row++
	case vkPrior:
		row -= fit
	case vkNext:
		row += fit
	case vkHome:
		row = 0
	case vkEnd:
		row = len(t.rows) - 1
	default:
		return false
	}
	c.pick(max(0, min(row, len(t.rows)-1)))
	return true
}

func (c *Control) drawTable(cv *Canvas) {
	w, t := c.w, c.table
	x, y, width, height := c.box()
	head, row, fit, bar := c.tableMeasures()
	radius, hair, inset := w.pxf(9), w.pxf(1), w.px(12)
	cv.Round(x, y, width, height, radius, theme.Well)
	cv.Ring(x, y, width, height, radius, hair, theme.WellEdge)
	_, length, _ := c.thumb()
	inner := width
	if length > 0 {
		inner -= bar
	}
	// Where each column starts. The last takes what is left.
	starts := make([]int, len(t.columns)+1)
	for i, column := range t.columns {
		starts[i+1] = starts[i] + w.px(column.Width)
	}
	starts[len(t.columns)] = inner
	cell := func(i int, text string, top, tall int, font uintptr, colour Colour) {
		if left, right := starts[i], min(starts[i+1], inner); right-left > 2*inset {
			w.write(cv, text, font, x+left+inset, top, right-left-inset, tall, textMiddle|textEllipsis, colour)
		}
	}
	for i, column := range t.columns {
		cell(i, column.Title, y, head, w.fonts.strong, theme.Soft)
	}
	cv.Fill(x+w.px(1), y+head, width-w.px(2), max(int(math.Round(hair)), 1), theme.Rule)
	for i := t.top; i < min(t.top+fit, len(t.rows)); i++ {
		top := y + head + w.px(2) + (i-t.top)*row
		if i == t.picked {
			cv.Round(x+w.px(4), top, inner-w.px(8), row, w.pxf(7), theme.RowChosen)
		}
		for j, text := range t.rows[i] {
			if j >= len(t.columns) {
				break
			}
			colour := theme.Ink
			if j == 0 && i < len(t.tints) && t.tints[i] != 0 {
				colour = t.tints[i]
			}
			cell(j, text, top, row, w.fonts.body, colour)
		}
	}
	if length > 0 {
		offset, _, _ := c.thumb()
		cv.Round(x+width-bar+w.px(3), y+head+w.px(4)+offset, bar-w.px(7), length, float64(bar-w.px(7))/2,
			pick(t.scrolling || w.hover == c, theme.SwitchEdge, theme.FieldEdge))
	}
}
