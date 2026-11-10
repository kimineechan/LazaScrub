//go:build windows

// Package winui provides a small native Win32 interface without third-party dependencies.
package winui

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	uxtheme  = syscall.NewLazyDLL("uxtheme.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	createWindow  = user32.NewProc("CreateWindowExW")
	sendMessage   = user32.NewProc("SendMessageW")
	postMessage   = user32.NewProc("PostMessageW")
	showWindow    = user32.NewProc("ShowWindow")
	enableWindow  = user32.NewProc("EnableWindow")
	setText       = user32.NewProc("SetWindowTextW")
	getText       = user32.NewProc("GetWindowTextW")
	getTextLength = user32.NewProc("GetWindowTextLengthW")
	loadImage     = user32.NewProc("LoadImageW")
	setBkColor    = gdi32.NewProc("SetBkColor")
	setTextColor  = gdi32.NewProc("SetTextColor")
)

// Window messages and notifications.
const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmSetRedraw   = 0x000B
	wmSetFont     = 0x0030
	wmNotify      = 0x004E
	wmCommand     = 0x0111
	wmColorEdit   = 0x0133
	wmColorButton = 0x0135
	wmColorStatic = 0x0138
	wmDropFiles   = 0x0233
	wmJobs        = 0x8001 // WM_APP + 1: run the functions queued by Post

	nmCustomDraw  = -12
	lvnItemChange = -101
)

// Colours, as 0x00BBGGRR.
const (
	colorWindow = 0x00FAFAFA
	colorField  = 0x00FFFFFF
	colorText   = 0x00202020
	colorMuted  = 0x00707070
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

type Window struct {
	Handle                       uintptr
	scale                        float64
	font, titleFont, headingFont uintptr
	brush, fieldBrush            uintptr
	pages                        map[int][]uintptr
	sections                     map[int]uintptr // the button that shows each page
	page                         int
	nextID                       uintptr
	actions                      map[uintptr]func()
	fields                       map[uintptr]bool // edit boxes, which are drawn white
	muted                        map[uintptr]bool // labels drawn in grey
	lists                        map[uintptr]*List
	drop                         func(paths []string)
	jobs                         []func()
	lock                         sync.Mutex
	callback                     uintptr
}

func wide(s string) []uint16     { return append(utf16.Encode([]rune(s)), 0) }
func ptr(s string) *uint16       { return &wide(s)[0] }
func (w *Window) px(n int) int32 { return int32(math.Round(float64(n) * w.scale)) }

// New creates the main window with a fixed layout of the given size.
func New(title, class string, width, height int) (*Window, error) {
	runtime.LockOSThread()
	user32.NewProc("SetProcessDPIAware").Call()
	controls := struct{ Size, Classes uint32 }{8, 0x1}
	comctl32.NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&controls)))
	dpi := uintptr(96)
	if p := user32.NewProc("GetDpiForSystem"); p.Find() == nil {
		dpi, _, _ = p.Call()
	}
	if dpi == 0 {
		dpi = 96
	}
	w := &Window{
		scale: float64(dpi) / 96, pages: map[int][]uintptr{}, sections: map[int]uintptr{}, actions: map[uintptr]func(){}, fields: map[uintptr]bool{},
		muted: map[uintptr]bool{}, lists: map[uintptr]*List{}, nextID: 100, page: -1,
	}
	screenWidth, _, _ := user32.NewProc("GetSystemMetrics").Call(0)
	screenHeight, _, _ := user32.NewProc("GetSystemMetrics").Call(1)
	// Keep the complete fixed layout visible even with a small screen or high scaling.
	w.scale = math.Min(w.scale, math.Min(float64(screenWidth-60)/float64(width), float64(screenHeight-100)/float64(height)))
	if w.scale < 0.7 {
		w.scale = 0.7
	}
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	w.brush, _, _ = gdi32.NewProc("CreateSolidBrush").Call(colorWindow)
	w.fieldBrush, _, _ = gdi32.NewProc("CreateSolidBrush").Call(colorField)
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	icon, _, _ := loadImage.Call(instance, 1, 1, 32, 32, 0)
	small, _, _ := loadImage.Call(instance, 1, 1, 16, 16, 0)
	w.callback = syscall.NewCallback(w.handle)
	className := ptr(class)
	wc := windowClass{Style: 3, Procedure: w.callback, Instance: instance, Icon: icon, SmallIcon: small, Cursor: cursor, Background: w.brush, ClassName: className}
	wc.Size = uint32(unsafe.Sizeof(wc))
	ok, _, e := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if ok == 0 {
		return nil, fmt.Errorf("could not register window: %w", e)
	}
	style := uintptr(0x00ca0000)
	r := rect{Right: w.px(width), Bottom: w.px(height)}
	user32.NewProc("AdjustWindowRectEx").Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	winWidth, winHeight := r.Right-r.Left, r.Bottom-r.Top
	x, y := (int32(screenWidth)-winWidth)/2, (int32(screenHeight)-winHeight)/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	h, _, e := createWindow.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(ptr(title))), style, uintptr(x), uintptr(y), uintptr(winWidth), uintptr(winHeight), 0, 0, instance, 0)
	if h == 0 {
		return nil, fmt.Errorf("could not create window: %w", e)
	}
	w.Handle = h
	w.font = w.makeFont(16, 400)
	w.titleFont = w.makeFont(30, 700)
	w.headingFont = w.makeFont(20, 600)
	return w, nil
}

// handle is the window procedure.
func (w *Window) handle(hwnd uintptr, msg uint32, wp uintptr, lp unsafe.Pointer) uintptr {
	switch msg {
	case wmCommand:
		if (wp>>16)&0xffff == 0 {
			if action := w.actions[wp&0xffff]; action != nil {
				action()
				return 0
			}
		}
	case wmNotify:
		if list := w.lists[(*notifyHeader)(lp).From]; list != nil {
			return list.notify(lp)
		}
	case wmColorStatic, wmColorEdit:
		if w.fields[uintptr(lp)] {
			setBkColor.Call(wp, colorField)
			return w.fieldBrush
		}
		setBkColor.Call(wp, colorWindow)
		if w.muted[uintptr(lp)] {
			setTextColor.Call(wp, colorMuted)
		} else {
			setTextColor.Call(wp, colorText)
		}
		return w.brush
	case wmColorButton:
		return w.brush
	case wmDropFiles:
		w.dropped(wp)
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
		user32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case wmDestroy:
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	result, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wp, uintptr(lp))
	return result
}

func (w *Window) makeFont(size, weight int) uintptr {
	height := w.px(-size)
	font, _, _ := gdi32.NewProc("CreateFontW").Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(ptr("Segoe UI"))))
	return font
}

func (w *Window) control(class, text string, style, extended uintptr, x, y, width, height int, action func()) uintptr {
	id := w.nextID
	w.nextID++
	h, _, _ := createWindow.Call(extended, uintptr(unsafe.Pointer(ptr(class))), uintptr(unsafe.Pointer(ptr(text))), 0x50000000|style, uintptr(w.px(x)), uintptr(w.px(y)), uintptr(w.px(width)), uintptr(w.px(height)), w.Handle, id, 0, 0)
	sendMessage.Call(h, wmSetFont, w.font, 1)
	if w.page >= 0 {
		w.pages[w.page] = append(w.pages[w.page], h)
	}
	if action != nil {
		w.actions[id] = action
	}
	return h
}

// Common makes the controls created next visible on every page.
func (w *Window) Common() { w.page = -1 }

// Page makes the controls created next belong to one page.
func (w *Window) Page(page int) { w.page = page }

// Section adds a button that shows a page. The button of the page on show stays pressed.
func (w *Window) Section(text string, page, x, y, width, height int) uintptr {
	h := w.control("BUTTON", text, 0x11004, 0, x, y, width, height, func() { w.ShowPage(page) })
	w.sections[page] = h
	return h
}

func (w *Window) ShowPage(page int) {
	for n, h := range w.sections {
		pressed := uintptr(0)
		if n == page {
			pressed = 1
		}
		sendMessage.Call(h, 0x00f1, pressed, 0)
	}
	for n, handles := range w.pages {
		for _, h := range handles {
			show := uintptr(0)
			if n == page {
				show = 5
			}
			showWindow.Call(h, show)
		}
	}
}

func (w *Window) Label(text string, x, y, width, height int) uintptr {
	return w.control("STATIC", text, 0x80, 0, x, y, width, height, nil)
}

// Hint is a label in a quieter colour, for secondary text.
func (w *Window) Hint(text string, x, y, width, height int) uintptr {
	h := w.Label(text, x, y, width, height)
	w.muted[h] = true
	return h
}

// Line is a one-line label. A path that is too long for it is shortened in
// the middle, so the file name at the end stays visible.
func (w *Window) Line(text string, x, y, width, height int) uintptr {
	return w.control("STATIC", text, 0x8080, 0, x, y, width, height, nil)
}

// Status is a Line in a quieter colour, for the bar at the foot of the window.
func (w *Window) Status(text string, x, y, width, height int) uintptr {
	h := w.Line(text, x, y, width, height)
	w.muted[h] = true
	return h
}

func (w *Window) Title(text string, x, y, width, height int) uintptr {
	h := w.Label(text, x, y, width, height)
	sendMessage.Call(h, wmSetFont, w.titleFont, 1)
	return h
}

func (w *Window) Heading(text string, x, y, width, height int) uintptr {
	h := w.Label(text, x, y, width, height)
	sendMessage.Call(h, wmSetFont, w.headingFont, 1)
	return h
}

// Edit is a single-line read-only box that shows a path.
func (w *Window) Edit(text string, x, y, width, height int) uintptr {
	h := w.control("EDIT", text, 0x10880, 0x200, x, y, width, height, nil)
	w.fields[h] = true
	return h
}

// Note is a read-only box that wraps its text over several lines.
func (w *Window) Note(text string, x, y, width, height int) uintptr {
	h := w.control("EDIT", text, 0x10844, 0x200, x, y, width, height, nil)
	w.fields[h] = true
	return h
}

func (w *Window) Button(text string, x, y, width, height int, action func()) uintptr {
	return w.control("BUTTON", text, 0x10000, 0, x, y, width, height, action)
}

func (w *Window) Checkbox(text string, x, y, width, height int, action func()) uintptr {
	return w.control("BUTTON", text, 0x10003, 0, x, y, width, height, action)
}

func (w *Window) Checked(h uintptr) bool { r, _, _ := sendMessage.Call(h, 0x00f0, 0, 0); return r == 1 }

func (w *Window) Text(h uintptr) string {
	n, _, _ := getTextLength.Call(h)
	buffer := make([]uint16, n+1)
	getText.Call(h, uintptr(unsafe.Pointer(&buffer[0])), n+1)
	return syscall.UTF16ToString(buffer)
}

func (w *Window) SetText(h uintptr, text string) { setText.Call(h, uintptr(unsafe.Pointer(ptr(text)))) }

// SetPath shows a path in an edit box, scrolled to its end so that the file name is visible.
func (w *Window) SetPath(h uintptr, path string) {
	w.SetText(h, path)
	end := uintptr(len(wide(path)))
	sendMessage.Call(h, 0x00B1, end, end)
	sendMessage.Call(h, 0x00B7, 0, 0)
}

func (w *Window) Enable(h uintptr, enabled bool) {
	b := uintptr(0)
	if enabled {
		b = 1
	}
	enableWindow.Call(h, b)
}

// OnDrop sets the function that receives files and folders dropped on the window.
func (w *Window) OnDrop(action func(paths []string)) {
	w.drop = action
	shell32.NewProc("DragAcceptFiles").Call(w.Handle, 1)
}

func (w *Window) dropped(drop uintptr) {
	query := shell32.NewProc("DragQueryFileW")
	count, _, _ := query.Call(drop, 0xFFFFFFFF, 0, 0)
	var paths []string
	for i := uintptr(0); i < count; i++ {
		buffer := make([]uint16, 32768)
		query.Call(drop, i, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		paths = append(paths, syscall.UTF16ToString(buffer))
	}
	shell32.NewProc("DragFinish").Call(drop)
	if w.drop != nil && len(paths) > 0 {
		w.drop(paths)
	}
}

func (w *Window) Alert(title, text string) {
	user32.NewProc("MessageBoxW").Call(w.Handle, uintptr(unsafe.Pointer(ptr(text))), uintptr(unsafe.Pointer(ptr(title))), 0x10)
}

func (w *Window) Info(title, text string) {
	user32.NewProc("MessageBoxW").Call(w.Handle, uintptr(unsafe.Pointer(ptr(text))), uintptr(unsafe.Pointer(ptr(title))), 0x40)
}

// Post runs a function on the window's thread. Background work uses it to update the interface.
func (w *Window) Post(job func()) {
	w.lock.Lock()
	w.jobs = append(w.jobs, job)
	w.lock.Unlock()
	postMessage.Call(w.Handle, wmJobs, 0, 0)
}

func (w *Window) Run() {
	showWindow.Call(w.Handle, 5)
	user32.NewProc("UpdateWindow").Call(w.Handle)
	var msg message
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		isDialog, _, _ := user32.NewProc("IsDialogMessageW").Call(w.Handle, uintptr(unsafe.Pointer(&msg)))
		if isDialog != 0 {
			continue
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	for _, object := range []uintptr{w.font, w.titleFont, w.headingFont, w.brush, w.fieldBrush} {
		gdi32.NewProc("DeleteObject").Call(object)
	}
}

type openFileName struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File                         *uint16
	MaxFile                      uint32
	FileTitle                    *uint16
	MaxFileTitle                 uint32
	InitialDir, Title            *uint16
	Flags                        uint32
	FileOffset, FileExtension    uint16
	DefaultExtension             *uint16
	CustomData, Hook             uintptr
	TemplateName                 *uint16
	Reserved                     uintptr
	ReservedDWORD, FlagsEx       uint32
}

// FileDialog asks for a file to open or a name to save under. It returns an
// empty path when the dialog is cancelled.
func (w *Window) FileDialog(title, filter, extension, suggested string, save bool) (string, error) {
	buffer := make([]uint16, 32768)
	copy(buffer, wide(suggested))
	filterData := wide(filter)
	config := openFileName{Owner: w.Handle, Filter: &filterData[0], File: &buffer[0], MaxFile: uint32(len(buffer)), Title: ptr(title), DefaultExtension: ptr(extension), FilterIndex: 1, Flags: 0x80000 | 0x800 | 0x8 | 0x4}
	config.Size = uint32(unsafe.Sizeof(config))
	proc := comdlg32.NewProc("GetOpenFileNameW")
	if save {
		proc = comdlg32.NewProc("GetSaveFileNameW")
		config.Flags |= 2
	} else {
		config.Flags |= 0x1000
	}
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&config)))
	runtime.KeepAlive(filterData)
	if ok == 0 {
		code, _, _ := comdlg32.NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return "", fmt.Errorf("file dialog error 0x%x", code)
		}
		return "", nil
	}
	return syscall.UTF16ToString(buffer), nil
}

// OpenFolder shows a folder in Explorer.
func (w *Window) OpenFolder(path string) {
	r, _, _ := shell32.NewProc("ShellExecuteW").Call(w.Handle, uintptr(unsafe.Pointer(ptr("open"))), uintptr(unsafe.Pointer(ptr(path))), 0, 0, 1)
	if r <= 32 {
		w.Alert("Could not open folder", fmt.Sprintf("Windows error %d", r))
	}
}

// Fatal reports an error that stops the app from starting.
func Fatal(title string, err error) {
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(ptr(err.Error()))), uintptr(unsafe.Pointer(ptr(title))), 0x10)
}
