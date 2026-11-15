//go:build windows

package winui

import (
	"unsafe"
)

var (
	notifyIcon      = shell32.NewProc("Shell_NotifyIconW")
	createPopupMenu = user32.NewProc("CreatePopupMenu")
	insertMenuItem  = user32.NewProc("InsertMenuItemW")
	setMenuInfo     = user32.NewProc("SetMenuInfo")
	trackPopupMenu  = user32.NewProc("TrackPopupMenu")
	destroyMenu     = user32.NewProc("DestroyMenu")
	getCursorPos    = user32.NewProc("GetCursorPos")
	systemMetrics   = user32.NewProc("GetSystemMetrics")
	textExtent      = gdi32.NewProc("GetTextExtentPoint32W")
	bitBlt          = gdi32.NewProc("BitBlt")
)

// notifyIconData mirrors NOTIFYICONDATAW.
type notifyIconData struct {
	Size            uint32
	Window          uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            [16]byte
	BalloonIcon     uintptr
}

// MenuEntry is one line of the menu of the notification area icon. An
// entry with no text is a dividing line.
type MenuEntry struct {
	Text   string
	Action func()
	Greyed bool
}

type tray struct {
	data    notifyIconData
	added   bool
	open    func()
	menu    func() []MenuEntry
	entries []MenuEntry // the menu on show
	lines   []menuLine
	surface *surface // where a line of the menu is drawn
	ground  uintptr  // the brush for the menu's own background
}

func fill(target []uint16, text string) {
	units := wide(text)
	if len(units) > len(target) {
		units = append(units[:len(target)-1], 0)
	}
	copy(target, units)
}

// Tray puts an icon in the notification area, next to the clock. icon is
// the number of an icon in the program's resources and tip the text shown
// when the mouse rests on it. open runs when the icon is clicked; menu
// supplies the entries shown on a right click.
func (w *Window) Tray(icon int, tip string, open func(), menu func() []MenuEntry) {
	w.tray = &tray{open: open, menu: menu}
	d := &w.tray.data
	d.Size = uint32(unsafe.Sizeof(*d))
	d.Window, d.ID, d.CallbackMessage = w.Handle, 1, wmTray
	w.SetTray(icon, tip)
}

// SetTray changes the icon and the tip.
func (w *Window) SetTray(icon int, tip string) {
	if w.tray == nil {
		return
	}
	width, _, _ := systemMetrics.Call(49)
	height, _, _ := systemMetrics.Call(50)
	d := &w.tray.data
	d.Icon, _, _ = loadImage.Call(w.instance, uintptr(icon), 1, width, height, 0)
	fill(d.Tip[:], tip)
	d.Flags = 0x1 | 0x2 | 0x4 // message, icon, tip
	w.tray.show(w)
}

func (t *tray) show(w *Window) {
	action := uintptr(1) // modify
	if !t.added {
		action = 0 // add
	}
	if ok, _, _ := notifyIcon.Call(action, uintptr(unsafe.Pointer(&t.data))); ok != 0 {
		t.added = true
	}
}

// Notify shows a short message by the notification area icon. Windows
// holds it back while the user has notifications silenced.
func (w *Window) Notify(title, text string) {
	if w.tray == nil || !w.tray.added {
		return
	}
	d := w.tray.data
	d.Flags |= 0x10 // with a message
	fill(d.InfoTitle[:], title)
	fill(d.Info[:], text)
	d.InfoFlags = 0x1 | 0x80 // information; respect quiet time
	notifyIcon.Call(1, uintptr(unsafe.Pointer(&d)))
}

// RemoveTray takes the icon away.
func (w *Window) RemoveTray() {
	if w.tray != nil && w.tray.added {
		notifyIcon.Call(2, uintptr(unsafe.Pointer(&w.tray.data)))
		w.tray.added = false
	}
}

// trayEvent handles a click on the icon.
func (w *Window) trayEvent(event uint32) {
	if w.tray == nil {
		return
	}
	switch event {
	case 0x0202, 0x0203, 0x0405: // left click, double click, click on a message
		if w.tray.open != nil {
			w.tray.open()
		}
	case 0x0205: // right click
		if w.tray.menu != nil && w.tray.entries == nil {
			w.popup(w.tray.menu())
		}
	}
}

// The menu is a real Windows menu, so the keyboard works in it as in any
// other, but its lines are drawn here, dark like the window.

// menuItem mirrors MENUITEMINFOW.
type menuItem struct {
	Size, Mask, Kind, State, ID uint32
	Sub, Checked, Unchecked     uintptr
	Data                        uintptr
	Text                        *uint16
	Length                      uint32
	Bitmap                      uintptr
}

// menuInfo mirrors MENUINFO.
type menuInfo struct {
	Size, Mask, Style, Tallest uint32
	Background                 uintptr
	Help                       uint32
	Data                       uintptr
}

// menuLine mirrors MSAAMENUINFO. Windows cannot tell a screen reader what
// a line says when a program draws the line itself; a screen reader looks
// for this beside each line instead.
type menuLine struct {
	Signature uint32
	Length    uint32
	Text      *uint16
}

// measureItem mirrors MEASUREITEMSTRUCT.
type measureItem struct {
	Kind, ID, Item, Width, Height uint32
	Data                          uintptr
}

// drawItem mirrors DRAWITEMSTRUCT.
type drawItem struct {
	Kind, ID, Item, Action, State uint32
	Window                        uintptr
	Context                       uintptr
	Area                          rect
	Data                          uintptr
}

// popup shows a menu at the mouse pointer and runs the entry chosen.
func (w *Window) popup(entries []MenuEntry) {
	menu, _, _ := createPopupMenu.Call()
	if menu == 0 {
		return
	}
	t := w.tray
	if t.ground == 0 {
		opaque := theme.List | 0xFF000000
		t.ground, _, _ = gdi32.NewProc("CreateSolidBrush").Call(uintptr(opaque&0xFF)<<16 | uintptr(opaque&0xFF00) | uintptr(opaque>>16&0xFF))
	}
	const background, style, noTicks = 0x2, 0x10, 0x80000000
	info := menuInfo{Mask: background | style, Style: noTicks, Background: t.ground}
	info.Size = uint32(unsafe.Sizeof(info))
	setMenuInfo.Call(menu, uintptr(unsafe.Pointer(&info)))

	t.entries, t.lines = entries, make([]menuLine, len(entries))
	for i, entry := range entries {
		const state, id, data, kind = 0x1, 0x2, 0x20, 0x100
		const drawnHere, divider, greyed = 0x100, 0x800, 0x3
		units := wide(entry.Text)
		t.lines[i] = menuLine{Signature: 0xAA0DF00D, Length: uint32(len(units) - 1), Text: &units[0]}
		item := menuItem{Mask: state | id | data | kind, Kind: drawnHere, ID: uint32(i + 1), Data: uintptr(unsafe.Pointer(&t.lines[i]))}
		item.Size = uint32(unsafe.Sizeof(item))
		if entry.Text == "" {
			item.Kind |= divider
		}
		if entry.Greyed {
			item.State = greyed
		}
		insertMenuItem.Call(menu, uintptr(i), 1, uintptr(unsafe.Pointer(&item)))
	}
	var at point
	getCursorPos.Call(uintptr(unsafe.Pointer(&at)))
	// Without this the menu would stay open when the user clicks elsewhere.
	setForeground.Call(w.Handle)
	const returnChoice, quiet, rightButton = 0x100, 0x80, 0x2
	chosen, _, _ := trackPopupMenu.Call(menu, returnChoice|quiet|rightButton, uintptr(at.X), uintptr(at.Y), 0, w.Handle, 0)
	postMessage.Call(w.Handle, 0, 0, 0)
	destroyMenu.Call(menu)
	t.entries, t.lines = nil, nil
	if chosen > 0 && int(chosen) <= len(entries) {
		if action := entries[chosen-1].Action; action != nil {
			action()
		}
	}
}

// menuEntry returns the entry of the menu on show that Windows asks about.
func (w *Window) menuEntry(kind, id uint32) *MenuEntry {
	const menu = 1
	if kind != menu || w.tray == nil || id == 0 || int(id) > len(w.tray.entries) {
		return nil
	}
	return &w.tray.entries[id-1]
}

// measureMenu tells Windows how large a line of the menu is.
func (w *Window) measureMenu(item *measureItem) bool {
	entry := w.menuEntry(item.Kind, item.Item)
	if entry == nil {
		return false
	}
	if entry.Text == "" {
		item.Width, item.Height = uint32(w.px(40)), uint32(w.px(9))
		return true
	}
	var size extent
	units := wide(entry.Text)
	selectObject.Call(w.stencil.context, w.fonts.body)
	textExtent.Call(w.stencil.context, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units)-1), uintptr(unsafe.Pointer(&size)))
	item.Width, item.Height = uint32(int(size.Width)+w.px(54)), uint32(w.px(32))
	return true
}

// drawMenu draws a line of the menu.
func (w *Window) drawMenu(item *drawItem) bool {
	entry := w.menuEntry(item.Kind, item.Item)
	if entry == nil {
		return false
	}
	t := w.tray
	width, height := int(item.Area.Right-item.Area.Left), int(item.Area.Bottom-item.Area.Top)
	if t.surface == nil || t.surface.width < width || t.surface.height < height {
		if t.surface != nil {
			t.surface.close()
		}
		var err error
		if t.surface, err = newSurface(max(width, w.px(320)), max(height, w.px(40))); err != nil {
			t.surface = nil
			return false
		}
	}
	cv := NewCanvas(t.surface.pix, t.surface.width, t.surface.height)
	cv.Clip(0, 0, width, height)
	cv.Set(0, 0, width, height, theme.List|0xFF000000)
	const selected, greyed = 0x1, 0x2
	switch {
	case entry.Text == "":
		cv.Fill(w.px(12), height/2, width-w.px(24), max(w.px(1), 1), theme.Rule)
	case item.State&greyed != 0:
		w.write(cv, entry.Text, w.fonts.body, w.px(18), 0, width-w.px(30), height, textMiddle, theme.Faint)
	default:
		if item.State&selected != 0 {
			cv.Round(w.px(4), w.px(1), width-w.px(8), height-w.px(2), w.pxf(7), theme.ListHover)
		}
		w.write(cv, entry.Text, w.fonts.body, w.px(18), 0, width-w.px(30), height, textMiddle, theme.Ink)
	}
	gdiFlush.Call()
	const copyAsIs = 0x00CC0020
	bitBlt.Call(item.Context, uintptr(item.Area.Left), uintptr(item.Area.Top), uintptr(width), uintptr(height), t.surface.context, 0, 0, copyAsIs)
	return true
}
