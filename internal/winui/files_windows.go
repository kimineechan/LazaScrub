//go:build windows

package winui

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

const wmDropFiles = 0x0233

// OnDrop sets the function called when files or folders are dropped on
// the window.
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

// Alert shows a message the user has to dismiss, in a box from Windows.
func (w *Window) Alert(title, text string) {
	user32.NewProc("MessageBoxW").Call(w.Handle, uintptr(unsafe.Pointer(ptr(text))), uintptr(unsafe.Pointer(ptr(title))), 0x10)
}

// openFileName mirrors OPENFILENAMEW.
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

// FileDialog asks for a file to open, or with save set for a name to save
// under, with the standard Windows dialog. It returns an empty path when
// the dialog is cancelled.
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
