//go:build windows

// Package winui provides a small native Win32 interface without third-party dependencies.
//
// The window is black and see-through, with rounded controls. Windows has
// no controls of that kind, so the package draws its own: the whole window
// is one picture in memory, redrawn only where something changed and handed
// to Windows, which blurs what is behind it. Nothing is drawn while the
// window is hidden or minimised.
package winui

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	createWindow    = user32.NewProc("CreateWindowExW")
	defWindowProc   = user32.NewProc("DefWindowProcW")
	postMessage     = user32.NewProc("PostMessageW")
	showWindow      = user32.NewProc("ShowWindow")
	loadImage       = user32.NewProc("LoadImageW")
	isWindowVisible = user32.NewProc("IsWindowVisible")
	isIconic        = user32.NewProc("IsIconic")
	setForeground   = user32.NewProc("SetForegroundWindow")
	getWindowRect   = user32.NewProc("GetWindowRect")
	setWindowPos    = user32.NewProc("SetWindowPos")
	getKeyState     = user32.NewProc("GetKeyState")
	setTimer        = user32.NewProc("SetTimer")
	killTimer       = user32.NewProc("KillTimer")
	setCapture      = user32.NewProc("SetCapture")
	releaseCapture  = user32.NewProc("ReleaseCapture")
	trackMouse      = user32.NewProc("TrackMouseEvent")
)

// Window messages.
const (
	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmActivate       = 0x0006
	wmClose          = 0x0010
	wmEraseBackdrop  = 0x0014
	wmDrawItem       = 0x002B
	wmMeasureItem    = 0x002C
	wmKeyDown        = 0x0100
	wmSysKeyDown     = 0x0104
	wmSysChar        = 0x0106
	wmSysCommand     = 0x0112
	wmTimer          = 0x0113
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmLButtonDouble  = 0x0203
	wmCaptureChanged = 0x0215
	wmEnterSizeMove  = 0x0231
	wmExitSizeMove   = 0x0232
	wmMouseWheel     = 0x020A
	wmMouseLeave     = 0x02A3
	wmHotkey         = 0x0312
	wmJobs           = 0x8001 // WM_APP + 1: run the functions queued by Post
	wmTray           = 0x8002 // WM_APP + 2: the notification area icon was used
	wmSignal         = 0x8003 // WM_APP + 3: another copy of the program asks for something
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type message struct {
	HWND           uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type windowClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}

// Window is the program's one window.
type Window struct {
	Handle        uintptr
	instance      uintptr
	scale         float64
	width, height int // in the units of the layout, before scaling

	canvas  *Canvas
	picture *surface // what the canvas draws on
	stencil *surface // where Windows draws text, to be coloured in on the canvas
	fonts   struct{ body, strong, small, heading, title uintptr }
	sheet   Colour  // the background in use
	frosted bool    // Windows blurs what is behind the window
	corner  float64 // how round the window's own corners are, in pixels
	build   uint32  // the build number of Windows

	controls []*Control
	page     int // the page controls are being added to, or -1 for every page
	current  int // the page on show
	grip     int // the window can be dragged by its top, down to here
	stale    rect
	drawn    bool // the window has been handed to Windows at least once

	hover, pressed, focus *Control
	keyboard              bool     // the focus was moved with the keyboard, so it is ringed
	active                bool     // the window is the one in front
	tracking              bool     // Windows will say when the pointer leaves
	dragging              bool     // the window is being moved by its top
	gripX, gripY          int      // the point of the window the pointer holds it by
	open                  *Control // the list that is showing its entries
	pick                  int      // the entry of that list the pointer or the keyboard is on

	shown          func(page int)
	drop           func(paths []string)
	tick           func()
	interval       int
	signal         func(command int)
	hotkey         func(id int)
	closing        func() bool
	tray           *tray
	taskbarCreated uint32
	jobs           []func()
	lock           sync.Mutex
	callback       uintptr
}

func wide(s string) []uint16 { return append(utf16.Encode([]rune(s)), 0) }
func ptr(s string) *uint16   { return &wide(s)[0] }

// px turns a length of the layout into pixels on this screen.
func (w *Window) px(n int) int          { return int(math.Round(float64(n) * w.scale)) }
func (w *Window) pxf(n float64) float64 { return n * w.scale }

// Signal sends a number to the window of a copy of the program that is
// already running, which receives it through OnSignal. It reports whether
// there was such a copy.
func Signal(class string, command int) bool {
	h, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(ptr(class))), 0)
	if h == 0 {
		return false
	}
	user32.NewProc("AllowSetForegroundWindow").Call(^uintptr(0)) // so it may come to the front
	postMessage.Call(h, wmSignal, uintptr(command), 0)
	return true
}

// OnSignal sets the function that receives numbers sent with Signal.
func (w *Window) OnSignal(signal func(command int)) { w.signal = signal }

// New creates the main window with a fixed layout of the given size. The
// window has no frame from Windows: it draws its own top, by which it can
// be dragged, and its own buttons to minimise and close.
func New(title, class string, width, height int) (*Window, error) {
	runtime.LockOSThread()
	user32.NewProc("SetProcessDPIAware").Call()
	dpi := uintptr(96)
	if p := user32.NewProc("GetDpiForSystem"); p.Find() == nil {
		dpi, _, _ = p.Call()
	}
	if dpi == 0 {
		dpi = 96
	}
	w := &Window{scale: float64(dpi) / 96, width: width, height: height, page: -1, pick: -1, build: windowsBuild()}
	// For trying the layout at another size without changing Windows' own
	// setting: LAZASCRUB_SCALE=150 draws everything at 150%.
	if percent, err := strconv.Atoi(os.Getenv("LAZASCRUB_SCALE")); err == nil && percent >= 50 && percent <= 400 {
		w.scale = float64(percent) / 100
	}
	screenWidth, _, _ := user32.NewProc("GetSystemMetrics").Call(0)
	screenHeight, _, _ := user32.NewProc("GetSystemMetrics").Call(1)
	// Keep the complete fixed layout visible even with a small screen or high scaling.
	w.scale = math.Min(w.scale, math.Min(float64(screenWidth-60)/float64(width), float64(screenHeight-100)/float64(height)))
	if w.scale < 0.7 {
		w.scale = 0.7
	}
	w.instance, _, _ = kernel32.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	icon, _, _ := loadImage.Call(w.instance, 1, 1, 32, 32, 0)
	small, _, _ := loadImage.Call(w.instance, 1, 1, 16, 16, 0)
	w.callback = syscall.NewCallback(w.handle)
	className := ptr(class)
	wc := windowClass{Procedure: w.callback, Instance: w.instance, Icon: icon, SmallIcon: small, Cursor: cursor, ClassName: className}
	wc.Size = uint32(unsafe.Sizeof(wc))
	ok, _, e := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if ok == 0 {
		return nil, fmt.Errorf("could not register window: %w", e)
	}
	// A window with no frame, on the taskbar, that can be minimised. "Layered"
	// is what lets each pixel have its own opacity.
	const popup, systemMenu, minimiseBox = 0x80000000, 0x00080000, 0x00020000
	const layered, onTaskbar = 0x00080000, 0x00040000
	winWidth, winHeight := w.px(width), w.px(height)
	x, y := max((int(screenWidth)-winWidth)/2, 0), max((int(screenHeight)-winHeight)/2, 0)
	h, _, e := createWindow.Call(layered|onTaskbar, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(ptr(title))),
		popup|systemMenu|minimiseBox, uintptr(x), uintptr(y), uintptr(winWidth), uintptr(winHeight), 0, 0, w.instance, 0)
	if h == 0 {
		return nil, fmt.Errorf("could not create window: %w", e)
	}
	w.Handle = h
	var err error
	if w.picture, err = newSurface(winWidth, winHeight); err != nil {
		return nil, err
	}
	if w.stencil, err = newSurface(winWidth, winHeight); err != nil {
		return nil, err
	}
	w.canvas = NewCanvas(w.picture.pix, winWidth, winHeight)
	w.fonts.body = w.makeFont(15, 400)
	w.fonts.strong = w.makeFont(15, 600)
	w.fonts.small = w.makeFont(13, 400)
	w.fonts.heading = w.makeFont(20, 600)
	w.fonts.title = w.makeFont(28, 700)
	w.backdrop()
	// Explorer announces itself with this message when it restarts, and the
	// notification area icon has to be put back.
	created, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(ptr("TaskbarCreated"))))
	w.taskbarCreated = uint32(created)
	return w, nil
}

// handle is the window procedure.
func (w *Window) handle(hwnd uintptr, msg uint32, wp uintptr, lp unsafe.Pointer) uintptr {
	// Whatever the message changed is redrawn once, when it has been dealt with.
	defer w.flush()
	switch msg {
	case wmEnterSizeMove: // the window is being moved from its menu, with the arrow keys
		if w.frosted {
			w.frost(true)
		}
	case wmExitSizeMove:
		if w.frosted {
			w.frost(false)
		}
	case wmMouseMove:
		x, y := w.position(lp)
		w.pointerMoved(x, y)
		return 0
	case wmMouseWheel:
		// The wheel turns the table the pointer is over.
		if c := w.hover; c != nil && c.kind == kindTable {
			c.scrollTo(c.table.top - 3*int(int16(wp>>16))/120)
		}
		return 0
	case wmDropFiles:
		w.dropped(wp)
		return 0
	case wmMouseLeave:
		w.tracking = false
		w.hoverOn(nil)
		return 0
	case wmLButtonDown, wmLButtonDouble:
		x, y := w.position(lp)
		w.pointerDown(x, y)
		return 0
	case wmLButtonUp:
		x, y := w.position(lp)
		w.pointerUp(x, y)
		return 0
	case wmCaptureChanged:
		if w.pressed != nil {
			w.pressed.redraw()
			w.pressed = nil
		}
		if w.dragging && uintptr(lp) != hwnd {
			w.letGo()
		}
		return 0
	case wmKeyDown, wmSysKeyDown:
		if w.keyDown(uint32(wp), uintptr(lp)&(1<<30) != 0, msg == wmSysKeyDown) {
			return 0
		}
	case wmSysChar:
		if w.setting() {
			return 0 // or Windows would beep at Alt with a letter
		}
	case wmSysCommand:
		if wp&0xFFF0 == 0xF100 && w.setting() {
			return 0 // Alt on its own would open the window's menu
		}
	case wmActivate:
		w.active = wp&0xFFFF != 0
		if !w.active {
			w.closeList()
		}
		if w.focus != nil {
			w.focus.redraw()
		}
	case wmSize:
		// Minimised, the window has nothing to show, so nothing is worked out.
		if wp == 1 {
			killTimer.Call(hwnd, 1)
		} else if wp == 0 && w.tick != nil {
			w.tick()
			setTimer.Call(hwnd, 1, uintptr(w.interval), 0)
			w.redrawAll()
		}
	case wmEraseBackdrop:
		return 1
	case wmMeasureItem:
		if w.measureMenu((*measureItem)(lp)) {
			return 1
		}
	case wmDrawItem:
		if w.drawMenu((*drawItem)(lp)) {
			return 1
		}
	case wmTimer:
		if w.tick != nil {
			w.tick()
		}
		return 0
	case wmHotkey:
		// A shortcut pressed while it is being chosen is not meant as a shortcut.
		if w.hotkey != nil && !(w.active && w.setting()) {
			w.hotkey(int(wp))
		}
		return 0
	case wmTray:
		w.trayEvent(uint32(uintptr(lp)))
		return 0
	case wmSignal:
		if w.signal != nil {
			w.signal(int(wp))
		}
		return 0
	case wmJobs:
		w.lock.Lock()
		jobs := w.jobs
		w.jobs = nil
		w.lock.Unlock()
		for _, job := range jobs {
			job()
		}
		return 0
	case wmClose:
		if w.closing != nil && !w.closing() {
			return 0
		}
		user32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case wmDestroy:
		w.RemoveTray()
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	if msg == w.taskbarCreated && w.tray != nil {
		w.tray.added = false
		w.tray.show(w)
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wp, uintptr(lp))
	return result
}

// position reads the two coordinates Windows packs into one number.
func (w *Window) position(lp unsafe.Pointer) (x, y int) {
	packed := uintptr(lp)
	return int(int16(packed)), int(int16(packed >> 16))
}

// held returns the modifier keys that are down.
func held() uint32 {
	var modifiers uint32
	for key, modifier := range map[uintptr]uint32{vkShift: Shift, vkControl: Ctrl, vkMenu: Alt} {
		if state, _, _ := getKeyState.Call(key); state&0x8000 != 0 {
			modifiers |= modifier
		}
	}
	return modifiers
}

// Page makes the controls created next belong to one page. Controls
// created before the first call are on show whichever page is.
func (w *Window) Page(page int) { w.page = page }

// ShowPage puts a page on show.
func (w *Window) ShowPage(page int) {
	w.closeList()
	w.current = page
	if w.focus != nil && !w.focus.showing() {
		w.focus = nil
	}
	w.hover = nil
	w.redrawAll()
	if w.shown != nil {
		w.shown(page)
	}
}

// OnPage sets a function that is called whenever a page is shown.
func (w *Window) OnPage(shown func(page int)) { w.shown = shown }

// Grip sets how far down from its top edge the window can be dragged by.
func (w *Window) Grip(height int) { w.grip = height }

// Every calls tick on the window's thread at the given interval in
// milliseconds for as long as the window is on show. A hidden or minimised
// window gets no calls at all, so it costs nothing; tick runs once more the
// moment the window is shown again.
func (w *Window) Every(milliseconds int, tick func()) {
	w.tick, w.interval = tick, milliseconds
	if w.Visible() {
		setTimer.Call(w.Handle, 1, uintptr(milliseconds), 0)
	}
}

// OnClose sets a function that is asked when the window is about to close.
// When it returns false the window stays.
func (w *Window) OnClose(closing func() bool) { w.closing = closing }

// SetHotkey registers a key combination that works whichever program is in
// front. id tells combinations apart when one is pressed.
func (w *Window) SetHotkey(id int, key, modifiers uint32) error {
	user32.NewProc("UnregisterHotKey").Call(w.Handle, uintptr(id))
	if key == 0 {
		return nil
	}
	const noRepeat = 0x4000
	ok, _, _ := user32.NewProc("RegisterHotKey").Call(w.Handle, uintptr(id), uintptr(modifiers|noRepeat), uintptr(key))
	if ok == 0 {
		return fmt.Errorf("another program is already using that shortcut")
	}
	return nil
}

// OnHotkey sets the function called when a registered combination is pressed.
func (w *Window) OnHotkey(pressed func(id int)) { w.hotkey = pressed }

// Show brings the window to the front, restoring it if it was hidden or minimised.
func (w *Window) Show() {
	if w.tick != nil {
		w.tick()
	}
	w.draw(w.whole()) // so that it appears complete, not as it was when hidden
	if minimised, _, _ := isIconic.Call(w.Handle); minimised != 0 {
		showWindow.Call(w.Handle, 9)
	} else {
		showWindow.Call(w.Handle, 5)
	}
	setForeground.Call(w.Handle)
	if w.tick != nil {
		setTimer.Call(w.Handle, 1, uintptr(w.interval), 0)
	}
}

// Hide takes the window off the screen and the taskbar.
func (w *Window) Hide() {
	killTimer.Call(w.Handle, 1)
	w.closeList()
	showWindow.Call(w.Handle, 0)
}

// Visible reports whether the window can be seen.
func (w *Window) Visible() bool {
	visible, _, _ := isWindowVisible.Call(w.Handle)
	minimised, _, _ := isIconic.Call(w.Handle)
	return visible != 0 && minimised == 0
}

// Close ends the program, whatever OnClose would say.
func (w *Window) Close() {
	w.closing = nil
	user32.NewProc("DestroyWindow").Call(w.Handle)
}

// Chime plays the plain notification sound of Windows.
func (w *Window) Chime() { user32.NewProc("MessageBeep").Call(0x40) }

// Warn plays the sound Windows uses when something needs attention.
func (w *Window) Warn() { user32.NewProc("MessageBeep").Call(0x30) }

// Post runs a function on the window's thread. Background work uses it to update the interface.
func (w *Window) Post(job func()) {
	w.lock.Lock()
	w.jobs = append(w.jobs, job)
	w.lock.Unlock()
	postMessage.Call(w.Handle, wmJobs, 0, 0)
}

// Run shows the window and handles its messages until it closes.
func (w *Window) Run() {
	w.Show()
	var msg message
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	for _, object := range []uintptr{w.fonts.body, w.fonts.strong, w.fonts.small, w.fonts.heading, w.fonts.title} {
		deleteObject.Call(object)
	}
	w.picture.close()
	w.stencil.close()
}

// OpenFolder shows a folder in Explorer.
func (w *Window) OpenFolder(path string) {
	r, _, _ := shell32.NewProc("ShellExecuteW").Call(w.Handle, uintptr(unsafe.Pointer(ptr("open"))), uintptr(unsafe.Pointer(ptr(path))), 0, 0, 1)
	if r <= 32 {
		w.Alert("Could not open folder", fmt.Sprintf("Windows error %d", r))
	}
}

// Reveal opens the folder of a file in Explorer with the file selected.
func (w *Window) Reveal(path string) {
	arguments := `/select,"` + path + `"`
	shell32.NewProc("ShellExecuteW").Call(w.Handle, uintptr(unsafe.Pointer(ptr("open"))), uintptr(unsafe.Pointer(ptr("explorer.exe"))), uintptr(unsafe.Pointer(ptr(arguments))), 0, 1)
}

// VideosFolder returns the user's Videos folder, or "" when Windows cannot say.
func VideosFolder() string {
	videos := guid{0x18989B1D, 0x99B5, 0x455B, [8]byte{0x84, 0x1C, 0xAB, 0x7C, 0x74, 0xE4, 0xDD, 0xFC}}
	var path *uint16
	r, _, _ := shell32.NewProc("SHGetKnownFolderPath").Call(uintptr(unsafe.Pointer(&videos)), 0, 0, uintptr(unsafe.Pointer(&path)))
	if r != 0 || path == nil {
		return ""
	}
	defer ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(path)))
	return syscall.UTF16ToString((*[32768]uint16)(unsafe.Pointer(path))[:])
}

// KeyName returns the name Windows gives a key, such as F9 or Page Up.
func KeyName(key uint32) string {
	switch {
	case key >= 0x70 && key <= 0x87:
		return fmt.Sprintf("F%d", key-0x6F)
	case key >= '0' && key <= '9', key >= 'A' && key <= 'Z':
		return string(rune(key))
	}
	scan, _, _ := user32.NewProc("MapVirtualKeyW").Call(uintptr(key), 0)
	if key >= 0x21 && key <= 0x2E {
		scan |= 0x100 // the arrow, paging and editing keys share codes with the number pad
	}
	buffer := make([]uint16, 64)
	if n, _, _ := user32.NewProc("GetKeyNameTextW").Call(scan<<16, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))); n > 0 {
		return syscall.UTF16ToString(buffer)
	}
	return fmt.Sprintf("key %d", key)
}

// Fatal reports an error that stops the app from starting.
func Fatal(title string, err error) {
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(ptr(err.Error()))), uintptr(unsafe.Pointer(ptr(title))), 0x10)
}
