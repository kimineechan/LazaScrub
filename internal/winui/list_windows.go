//go:build windows

package winui

import (
	"unsafe"
)

// List view messages, styles and notification stages.
const (
	lvmDeleteAll   = 0x1009
	lvmSetStyle    = 0x1036
	lvmInsertItem  = 0x104D
	lvmInsertCol   = 0x1061
	lvmSetItemText = 0x1074

	drawList     = 0x00001 // the list is about to be painted
	drawRow      = 0x10001 // a row is about to be painted
	drawCell     = 0x30001 // a cell is about to be painted
	askPerItem   = 0x20
	useNewColour = 0x2
	defaultColor = 0xFF000000
)

// Column is one column of a List.
type Column struct {
	Title string
	Width int
}

// List is a table with a header, one selectable row at a time.
type List struct {
	Handle   uintptr
	columns  int
	tints    []uint32
	onSelect func(row int)
}

type notifyHeader struct {
	From, ID uintptr
	Code     int32
}

type listColumn struct {
	Mask                uint32
	Format, Width       int32
	Text                *uint16
	TextMax, SubItem    int32
	Image, Order        int32
	Min, Default, Ideal int32
}

type listItem struct {
	Mask             uint32
	Item, SubItem    int32
	State, StateMask uint32
	Text             *uint16
	TextMax, Image   int32
	Param            uintptr
	Indent, GroupID  int32
	Columns          uint32
	ColumnIDs        *uint32
	ColumnFormats    *int32
	Group            int32
}

type listChange struct {
	Header             notifyHeader
	Item, SubItem      int32
	NewState, OldState uint32
	Changed            uint32
	Action             point
	Param              uintptr
}

type listDraw struct {
	Header     notifyHeader
	Stage      uint32
	DC         uintptr
	Area       rect
	Item       uintptr
	State      uint32
	Param      uintptr
	Text, Back uint32
	SubItem    int32
}

// List creates a table. onSelect is called with the row the user picks.
func (w *Window) List(x, y, width, height int, columns []Column, onSelect func(row int)) *List {
	h := w.control("SysListView32", "", 0x1800D, 0x200, x, y, width, height, nil)
	sendMessage.Call(h, lvmSetStyle, 0, 0x14020)
	uxtheme.NewProc("SetWindowTheme").Call(h, uintptr(unsafe.Pointer(ptr("Explorer"))), 0)
	for i, c := range columns {
		column := listColumn{Mask: 0x7, Width: w.px(c.Width), Text: ptr(c.Title)}
		sendMessage.Call(h, lvmInsertCol, uintptr(i), uintptr(unsafe.Pointer(&column)))
	}
	list := &List{Handle: h, columns: len(columns), onSelect: onSelect}
	w.lists[h] = list
	return list
}

func clip(s string) *uint16 {
	if r := []rune(s); len(r) > 400 {
		s = string(r[:400])
	}
	return ptr(s)
}

// Fill replaces every row. A tint, when given, colours the first cell of its row.
func (l *List) Fill(rows [][]string, tints []uint32) {
	sendMessage.Call(l.Handle, wmSetRedraw, 0, 0)
	sendMessage.Call(l.Handle, lvmDeleteAll, 0, 0)
	l.tints = tints
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		item := listItem{Mask: 1, Item: int32(i), Text: clip(row[0])}
		sendMessage.Call(l.Handle, lvmInsertItem, 0, uintptr(unsafe.Pointer(&item)))
		for j := 1; j < len(row) && j < l.columns; j++ {
			l.Set(i, j, row[j])
		}
	}
	sendMessage.Call(l.Handle, wmSetRedraw, 1, 0)
	user32.NewProc("InvalidateRect").Call(l.Handle, 0, 1)
}

// Set changes the text of one cell.
func (l *List) Set(row, column int, text string) {
	item := listItem{SubItem: int32(column), Text: clip(text)}
	sendMessage.Call(l.Handle, lvmSetItemText, uintptr(row), uintptr(unsafe.Pointer(&item)))
}

// notify handles the messages the list sends to its parent window.
func (l *List) notify(lp unsafe.Pointer) uintptr {
	switch (*notifyHeader)(lp).Code {
	case lvnItemChange:
		change := (*listChange)(lp)
		if change.NewState&2 != 0 && change.OldState&2 == 0 && l.onSelect != nil {
			l.onSelect(int(change.Item))
		}
	case nmCustomDraw:
		draw := (*listDraw)(lp)
		switch draw.Stage {
		case drawList, drawRow:
			return askPerItem
		case drawCell:
			draw.Text = defaultColor
			if row := int(draw.Item); draw.SubItem == 0 && row < len(l.tints) && l.tints[row] != 0 {
				draw.Text = l.tints[row]
			}
			return useNewColour
		}
	}
	return 0
}
