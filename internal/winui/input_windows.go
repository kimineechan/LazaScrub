//go:build windows

package winui

import "unsafe"

// at returns the control under a point of the window that the pointer can
// work, or nil.
func (w *Window) at(x, y int) *Control {
	for i := len(w.controls) - 1; i >= 0; i-- {
		c := w.controls[i]
		if !c.usable() {
			continue
		}
		if cx, cy, width, height := c.box(); x >= cx && x < cx+width && y >= cy && y < cy+height {
			return c
		}
	}
	return nil
}

func (w *Window) hoverOn(c *Control) {
	if w.hover == c {
		return
	}
	if w.hover != nil {
		w.hover.redraw()
	}
	if w.hover = c; c != nil {
		c.redraw()
	}
}

// focusOn moves the keyboard to a control, or to none.
func (w *Window) focusOn(c *Control) {
	if w.focus == c {
		return
	}
	if w.focus != nil {
		w.focus.redraw()
	}
	if w.focus = c; c != nil {
		c.redraw()
	}
}

// setting reports whether the keyboard is in a shortcut box, where keys
// are taken down instead of obeyed.
func (w *Window) setting() bool { return w.focus != nil && w.focus.kind == kindShortcut }

func (w *Window) pointerMoved(x, y int) {
	if !w.tracking {
		// Ask to be told when the pointer leaves, so the control it was
		// last over does not stay lit.
		request := struct {
			Size, Flags uint32
			Window      uintptr
			Hover       uint32
		}{Flags: 2, Window: w.Handle}
		request.Size = uint32(unsafe.Sizeof(request))
		trackMouse.Call(uintptr(unsafe.Pointer(&request)))
		w.tracking = true
	}
	if w.dragging {
		w.drag(x, y)
		return
	}
	if c := w.pressed; c != nil && c.kind == kindTable && c.table.scrolling {
		c.tableDrag(y)
		return
	}
	if w.open != nil {
		if entry := w.entryAt(x, y); entry >= 0 && entry != w.pick {
			w.pick = entry
			w.redrawList()
		}
		return
	}
	w.hoverOn(w.at(x, y))
}

func (w *Window) pointerDown(x, y int) {
	if w.keyboard {
		w.keyboard = false
		if w.focus != nil {
			w.focus.redraw()
		}
	}
	if w.open != nil {
		// A press on an entry chooses it; a press anywhere else only puts
		// the list away.
		if entry := w.entryAt(x, y); entry >= 0 {
			w.choose(entry)
		} else {
			w.closeList()
		}
		return
	}
	c := w.at(x, y)
	w.hoverOn(c)
	if c == nil || !c.reachable() {
		w.focusOn(nil)
	} else {
		w.focusOn(c)
	}
	if w.pressed = c; c != nil {
		setCapture.Call(w.Handle) // the release comes here even if the pointer has left the window
		c.redraw()
		if c.kind == kindTable {
			c.tableDown(x, y)
		}
	} else if y < w.px(w.grip) {
		// The top of the window stands in for a title bar: held there, the
		// window follows the pointer until the button is let go.
		w.dragging, w.gripX, w.gripY = true, x, y
		setCapture.Call(w.Handle)
		if w.frosted {
			w.frost(true)
		}
	}
}

// drag moves the window so that the point it was taken hold of stays under
// the pointer.
func (w *Window) drag(x, y int) {
	var at rect
	getWindowRect.Call(w.Handle, uintptr(unsafe.Pointer(&at)))
	const keepSize, keepOrder, keepInactive = 0x1, 0x4, 0x10
	setWindowPos.Call(w.Handle, 0, uintptr(int(at.Left)+x-w.gripX), uintptr(int(at.Top)+y-w.gripY), 0, 0, keepSize|keepOrder|keepInactive)
}

func (w *Window) letGo() {
	if w.dragging {
		w.dragging = false
		releaseCapture.Call()
		if w.frosted {
			w.frost(false)
		}
	}
}

func (w *Window) pointerUp(x, y int) {
	w.letGo()
	c := w.pressed
	if c == nil {
		return
	}
	w.pressed = nil
	releaseCapture.Call()
	c.redraw()
	if c.kind == kindTable {
		c.table.scrolling = false
		return
	}
	if w.at(x, y) == c {
		c.press()
	}
}

// press does what a click on the control does.
func (c *Control) press() {
	w := c.w
	switch c.kind {
	case kindSection:
		if w.current != c.target {
			w.ShowPage(c.target)
		}
	case kindSwitch:
		c.on = !c.on
		c.redraw()
		c.run()
	case kindChoice:
		if w.open == c {
			w.closeList()
		} else {
			w.openList(c)
		}
	case kindShortcut:
		// Having the keyboard is all a shortcut box needs.
	default:
		c.run()
	}
}

func (c *Control) run() {
	if c.action != nil {
		c.action()
	}
}

// keyDown handles a key and reports whether it was used. system is set for
// keys pressed with Alt, which Windows would otherwise act on itself.
func (w *Window) keyDown(key uint32, repeat, system bool) bool {
	modifiers := held()
	back := modifiers&Shift != 0
	if w.open != nil {
		switch key {
		case vkEscape:
			w.closeList()
		case vkReturn, vkSpace:
			w.choose(w.pick)
		case vkTab:
			w.closeList()
			w.focusNext(back)
		default:
			if entry, ok := step(key, w.pick, len(w.open.entries)); ok && entry != w.pick {
				w.pick = entry
				w.redrawList()
			}
		}
		return true
	}
	if c := w.focus; w.setting() && key != vkTab {
		switch action, with := combination(key, modifiers); action {
		case keyClears:
			c.SetKeys(0, 0)
			c.run()
		case keySets:
			c.SetKeys(key, with)
			c.run()
		}
		// Alt with F4 still closes the window. Every other key stays here.
		return !(system && key == vkF4)
	}
	if system {
		// Alt with an arrow opens a list, as it does everywhere in Windows.
		if c := w.focus; c != nil && c.kind == kindChoice && (key == vkDown || key == vkUp) {
			w.keyboard = true
			w.openList(c)
			return true
		}
		return false
	}
	c := w.focus
	if c != nil && c.kind == kindTable && c.tableKey(key) {
		w.keyboard = true
		return true
	}
	switch key {
	case vkTab:
		if modifiers&Ctrl != 0 {
			w.pageNext(back)
		} else {
			w.focusNext(back)
		}
	case vkReturn, vkSpace:
		if c != nil && !repeat {
			w.keyboard = true
			c.press()
		}
	case vkLeft, vkRight, vkUp, vkDown, vkF4:
		switch {
		case c == nil:
			return false
		case c.kind == kindChoice && key != vkLeft && key != vkRight:
			w.keyboard = true
			w.openList(c)
		case c.kind == kindSection:
			w.pageNext(key == vkLeft || key == vkUp)
		}
	default:
		return false
	}
	return true
}

// focusNext moves the keyboard to the next control, or the one before.
func (w *Window) focusNext(back bool) {
	var stops []*Control
	current := -1
	for _, c := range w.controls {
		if c.reachable() {
			if c == w.focus {
				current = len(stops)
			}
			stops = append(stops, c)
		}
	}
	if len(stops) == 0 {
		return
	}
	next := 0
	switch {
	case current < 0 && back:
		next = len(stops) - 1
	case current >= 0 && back:
		next = (current + len(stops) - 1) % len(stops)
	case current >= 0:
		next = (current + 1) % len(stops)
	}
	if !w.keyboard {
		w.keyboard = true
		if w.focus != nil {
			w.focus.redraw()
		}
	}
	w.focusOn(stops[next])
}

// pageNext shows the next page, or the one before, and puts the keyboard
// on its section button.
func (w *Window) pageNext(back bool) {
	var sections []*Control
	current := 0
	for _, c := range w.controls {
		if c.kind == kindSection {
			if c.target == w.current {
				current = len(sections)
			}
			sections = append(sections, c)
		}
	}
	if len(sections) == 0 {
		return
	}
	next := (current + 1) % len(sections)
	if back {
		next = (current + len(sections) - 1) % len(sections)
	}
	w.keyboard = true
	w.ShowPage(sections[next].target)
	w.focusOn(sections[next])
}

// The entries of an open list are drawn over the window, below the list's
// box, or above it when there is no room below.

// listBox returns where the entries of the open list are, in pixels, and
// the height of one entry.
func (w *Window) listBox() (x, y, width, height, row int) {
	c := w.open
	x, top, width, tall := c.box()
	row = w.px(32)
	height = len(c.entries)*row + 2*w.px(4)
	y = top + tall + w.px(4)
	if y+height > w.px(w.height)-w.px(8) {
		y = top - w.px(4) - height
	}
	return x, y, width, height, row
}

func (w *Window) redrawList() {
	if w.open != nil {
		x, y, width, height, _ := w.listBox()
		w.redraw(x, y, width, height)
	}
}

// entryAt returns the entry of the open list under a point, or -1.
func (w *Window) entryAt(px, py int) int {
	x, y, width, _, row := w.listBox()
	inset := w.px(4)
	if px < x+inset || px >= x+width-inset || py < y+inset {
		return -1
	}
	if entry := (py - y - inset) / row; entry < len(w.open.entries) {
		return entry
	}
	return -1
}

func (w *Window) openList(c *Control) {
	if len(c.entries) == 0 {
		return
	}
	w.closeList()
	w.hoverOn(nil)
	w.open, w.pick = c, max(c.chosen, 0)
	c.redraw()
	w.redrawList()
}

func (w *Window) closeList() {
	if c := w.open; c != nil {
		w.redrawList()
		w.open = nil
		c.redraw()
	}
}

// choose takes an entry of the open list as the choice.
func (w *Window) choose(entry int) {
	c := w.open
	w.closeList()
	if entry >= 0 && entry < len(c.entries) && entry != c.chosen {
		c.chosen = entry
		c.redraw()
		c.run()
	}
}

func (w *Window) drawList(cv *Canvas) {
	c := w.open
	x, y, width, height, row := w.listBox()
	cv.Round(x, y, width, height, w.pxf(10), theme.List)
	cv.Ring(x, y, width, height, w.pxf(10), w.pxf(1), theme.ListEdge)
	inset := w.px(4)
	for i, entry := range c.entries {
		top := y + inset + i*row
		if i == w.pick {
			cv.Round(x+inset, top, width-2*inset, row, w.pxf(7), theme.ListHover)
		}
		colour := theme.Soft
		if i == c.chosen {
			colour = theme.Ink
			// A tick marks the entry in use.
			tx, ty, arm := float64(x+width)-w.pxf(22), float64(top)+float64(row)/2, w.pxf(4)
			cv.Line(tx-arm, ty, tx-arm/4, ty+arm*3/4, w.pxf(1.5), theme.Ink)
			cv.Line(tx-arm/4, ty+arm*3/4, tx+arm, ty-arm*3/4, w.pxf(1.5), theme.Ink)
		} else if i == w.pick {
			colour = theme.Ink
		}
		w.write(cv, entry, w.fonts.body, x+inset+w.px(10), top, width-2*inset-w.px(38), row, textMiddle|textEllipsis, colour)
	}
}
