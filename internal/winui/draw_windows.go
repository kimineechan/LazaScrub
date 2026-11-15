//go:build windows

package winui

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	selectObject       = gdi32.NewProc("SelectObject")
	deleteObject       = gdi32.NewProc("DeleteObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
	gdiFlush           = gdi32.NewProc("GdiFlush")
	setTextColor       = gdi32.NewProc("SetTextColor")
	setBkMode          = gdi32.NewProc("SetBkMode")
	drawText           = user32.NewProc("DrawTextW")
	updateLayered      = user32.NewProc("UpdateLayeredWindowIndirect")
	setComposition     = user32.NewProc("SetWindowCompositionAttribute")
)

// surface is a picture in memory that Windows can draw on and copy from.
type surface struct {
	context, bitmap uintptr
	width, height   int
	pix             []uint32
}

// bitmapHeader mirrors BITMAPINFOHEADER.
type bitmapHeader struct {
	Size                 uint32
	Width, Height        int32
	Planes, Bits         uint16
	Compression          uint32
	ImageSize            uint32
	XPerMetre, YPerMetre int32
	Used, Important      uint32
}

func newSurface(width, height int) (*surface, error) {
	// A negative height puts the first row at the top.
	header := bitmapHeader{Size: 40, Width: int32(width), Height: -int32(height), Planes: 1, Bits: 32}
	s := &surface{width: width, height: height}
	s.context, _, _ = createCompatibleDC.Call(0)
	var bits unsafe.Pointer
	s.bitmap, _, _ = createDIBSection.Call(s.context, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if s.context == 0 || s.bitmap == 0 || bits == nil {
		return nil, fmt.Errorf("could not make a picture of %d by %d to draw the window on", width, height)
	}
	selectObject.Call(s.context, s.bitmap)
	setBkMode.Call(s.context, 1) // text is drawn without a box behind it
	s.pix = unsafe.Slice((*uint32)(bits), width*height)
	return s, nil
}

func (s *surface) close() {
	deleteDC.Call(s.context)
	deleteObject.Call(s.bitmap)
}

func (w *Window) makeFont(size, weight int) uintptr {
	const clearType = 5
	font, _, _ := gdi32.NewProc("CreateFontW").Call(uintptr(-w.px(size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, clearType, 0, uintptr(unsafe.Pointer(ptr("Segoe UI"))))
	return font
}

// How text is placed in its rectangle.
const (
	textCentre   = 0x1        // between left and right
	textMiddle   = 0x4 | 0x20 // one line, between top and bottom
	textWrap     = 0x10       // on as many lines as it needs
	textEllipsis = 0x8000     // cut short with an ellipsis when too long
	textPath     = 0x4000     // cut short in the middle, keeping the file name
	textAsIs     = 0x800      // an ampersand is an ampersand
)

// write draws text in a rectangle of a canvas, given in pixels.
//
// Windows draws the letters, but its text drawing knows nothing of
// opacity and would punch holes in a see-through window. So the letters go
// on a picture of their own, white on black, and that picture is used as a
// stencil through which the colour is laid on the canvas. Dark text is
// drawn black on white, so that its edges are shaped for a light ground.
func (w *Window) write(cv *Canvas, text string, font uintptr, x, y, width, height int, flags uint32, colour Colour) {
	width, height = min(width, w.stencil.width), min(height, w.stencil.height)
	if text == "" || width <= 0 || height <= 0 {
		return
	}
	dark := colour>>16&0xFF+colour>>8&0xFF+colour&0xFF < 384
	ground := uint32(0)
	if dark {
		ground = 0xFFFFFF
	}
	for row := 0; row < height; row++ {
		line := w.stencil.pix[row*w.stencil.width:][:width]
		for i := range line {
			line[i] = ground
		}
	}
	selectObject.Call(w.stencil.context, font)
	setTextColor.Call(w.stencil.context, uintptr(ground^0xFFFFFF))
	area := rect{0, 0, int32(width), int32(height)}
	drawText.Call(w.stencil.context, uintptr(unsafe.Pointer(ptr(text))), ^uintptr(0), uintptr(unsafe.Pointer(&area)), uintptr(flags|textAsIs))
	gdiFlush.Call()
	cv.Stencil(x, y, w.stencil.pix, w.stencil.width, width, height, colour, dark)
}

// redraw marks a rectangle of the window, in pixels, as out of date.
func (w *Window) redraw(x, y, width, height int) {
	area := rect{int32(x), int32(y), int32(x + width), int32(y + height)}
	if w.stale.Right <= w.stale.Left {
		w.stale = area
		return
	}
	w.stale = rect{min(w.stale.Left, area.Left), min(w.stale.Top, area.Top), max(w.stale.Right, area.Right), max(w.stale.Bottom, area.Bottom)}
}

func (w *Window) whole() rect { return rect{0, 0, int32(w.px(w.width)), int32(w.px(w.height))} }

func (w *Window) redrawAll() { w.stale = w.whole() }

// flush redraws what is out of date, if the window can be seen. While it
// is hidden or minimised nothing is drawn; Show draws the whole of it.
func (w *Window) flush() {
	if w.stale.Right <= w.stale.Left || w.canvas == nil || !w.Visible() {
		return
	}
	w.draw(w.stale)
}

// draw redraws a rectangle of the window and hands the result to Windows.
func (w *Window) draw(area rect) {
	w.stale = rect{}
	c := w.canvas
	if !c.Clip(int(area.Left), int(area.Top), int(area.Right), int(area.Bottom)) {
		return
	}
	c.Set(0, 0, c.Width, c.Height, w.sheet)
	c.Ring(0, 0, c.Width, c.Height, w.corner, w.pxf(1), theme.Edge)
	for _, control := range w.controls {
		if control.showing() && control.touches(area) {
			control.draw(c)
		}
	}
	if w.open != nil {
		w.drawList(c)
	}
	w.present(area)
}

type extent struct{ Width, Height int32 }

// blending mirrors BLENDFUNCTION.
type blending struct{ Operation, Flags, Opacity, Format byte }

// update mirrors UPDATELAYEREDWINDOWINFO.
type update struct {
	Size     uint32
	Target   uintptr
	Position *point
	Extent   *extent
	Source   uintptr
	From     *point
	Key      uint32
	Blend    *blending
	Flags    uint32
	Dirty    *rect
}

// present hands the picture to Windows, which keeps its own copy and shows
// it with each pixel as see-through as the picture says. Only the changed
// rectangle is copied. Because Windows keeps the copy, uncovering or moving
// the window costs this program nothing.
func (w *Window) present(area rect) {
	const perPixel = 1 // AC_SRC_ALPHA
	const useAlpha = 2 // ULW_ALPHA
	size, from := extent{int32(w.picture.width), int32(w.picture.height)}, point{}
	info := update{Extent: &size, Source: w.picture.context, From: &from, Blend: &blending{Opacity: 255, Format: perPixel}, Flags: useAlpha}
	info.Size = uint32(unsafe.Sizeof(info))
	if w.drawn {
		info.Dirty = &area
	}
	gdiFlush.Call()
	if ok, _, _ := updateLayered.Call(w.Handle, uintptr(unsafe.Pointer(&info))); ok != 0 {
		w.drawn = true
	}
}

// accent mirrors ACCENT_POLICY, which SetWindowCompositionAttribute takes.
type accent struct {
	State, Flags int32
	Colour       uint32
	Animation    int32
}

// compositionData mirrors WINDOWCOMPOSITIONATTRIBDATA.
type compositionData struct {
	Attribute uint32
	Data      unsafe.Pointer
	Size      uintptr
}

// backdrop decides how see-through the window is and asks Windows to blur
// what is behind it.
//
// A window that only lets the desktop show through is hard to read over a
// page of text. Blurred, the same desktop gives colour without detail, and
// the window can let more of it through. Where Windows cannot blur, less
// is let through instead, and with high contrast turned on, nothing.
func (w *Window) backdrop() {
	var contrast struct {
		Size, Flags uint32
		Scheme      *uint16
	}
	contrast.Size = uint32(unsafe.Sizeof(contrast))
	user32.NewProc("SystemParametersInfoW").Call(0x42, uintptr(contrast.Size), uintptr(unsafe.Pointer(&contrast)), 0)
	switch {
	case contrast.Flags&1 != 0:
		w.sheet = theme.SheetSolid
	case w.frost(false):
		w.sheet, w.frosted = theme.Sheet, true
	default:
		w.sheet = theme.SheetPlain
	}
	if w.build >= 22000 {
		// Windows 11 rounds the corners of windows when asked. Windows 10
		// has square windows, and so does this one there.
		const cornerPreference, round = 33, 2
		preference := uint32(round)
		dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
		if r, _, _ := dwm.Call(w.Handle, cornerPreference, uintptr(unsafe.Pointer(&preference)), 4); r == 0 {
			w.corner = w.pxf(8)
		}
	}
}

// frost turns on the blur behind the window and reports whether Windows
// took the request.
//
// Windows has two blurs. The stronger, the frosted glass of its own Start
// menu, came with the April 2018 version of Windows 10. From the May 2019
// version until Windows 11 a window with it trails behind the mouse while
// being dragged, so the weaker blur stands in for as long as a drag lasts.
// The call is one Windows does not document, though it has kept its form
// since 2015; if it is missing or refuses, the window does without.
func (w *Window) frost(moving bool) bool {
	if setComposition.Find() != nil {
		return false
	}
	const accentPolicy, blur, glass = 19, 3, 4
	states := []int32{glass, blur}
	if w.build < 17134 || (moving && w.build >= 18362 && w.build < 22000) {
		states = states[1:]
	}
	for _, state := range states {
		// The colour is a tint Windows adds to the glass. The canvas brings
		// its own, so this one is as good as clear; fully clear is refused.
		policy := accent{State: state, Colour: 0x01000000}
		data := compositionData{Attribute: accentPolicy, Data: unsafe.Pointer(&policy), Size: unsafe.Sizeof(policy)}
		if ok, _, _ := setComposition.Call(w.Handle, uintptr(unsafe.Pointer(&data))); ok != 0 {
			return true
		}
	}
	return false
}

// windowsBuild returns the build number of Windows, such as 19045.
func windowsBuild() uint32 {
	var version struct {
		Size, Major, Minor, Build, Platform uint32
		ServicePack                         [128]uint16
	}
	version.Size = uint32(unsafe.Sizeof(version))
	syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&version)))
	return version.Build
}
