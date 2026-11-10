//go:build windows

package winui

import (
	"fmt"
	"syscall"
	"unsafe"
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	classFileOpenDialog     = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	interfaceFileOpenDialog = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

// Positions of the methods used in the IFileOpenDialog and IShellItem tables.
const (
	methodRelease     = 2
	methodShow        = 3
	methodDisplayName = 5
	methodSetOptions  = 9
	methodSetTitle    = 17
	methodGetResult   = 20
)

const (
	pickExistingFolder = 0x20 | 0x40 | 0x800 // folders only, on a real file system, that exist
	fileSystemPath     = 0x80058000
	cancelled          = 0x800704C7
	modeChanged        = 0x80010106 // COM is already running on this thread in another mode
)

// call invokes a method of a COM object through its method table.
func call(object unsafe.Pointer, method int, args ...uintptr) uintptr {
	table := *(**[32]uintptr)(object)
	r, _, _ := syscall.SyscallN(table[method], append([]uintptr{uintptr(object)}, args...)...)
	return r
}

// FolderDialog asks for a folder with the standard Windows picker. It
// returns an empty path when the dialog is cancelled.
func (w *Window) FolderDialog(title string) (string, error) {
	switch r, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2); uint32(r) {
	case 0, 1:
		defer ole32.NewProc("CoUninitialize").Call()
	case modeChanged:
	default:
		return "", fmt.Errorf("could not start the folder picker (error 0x%x)", r)
	}
	var dialog unsafe.Pointer
	r, _, _ := ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&classFileOpenDialog)), 0, 1,
		uintptr(unsafe.Pointer(&interfaceFileOpenDialog)), uintptr(unsafe.Pointer(&dialog)))
	if r != 0 || dialog == nil {
		return "", fmt.Errorf("could not open the folder picker (error 0x%x)", r)
	}
	defer call(dialog, methodRelease)
	call(dialog, methodSetOptions, pickExistingFolder)
	call(dialog, methodSetTitle, uintptr(unsafe.Pointer(ptr(title))))
	switch r := call(dialog, methodShow, w.Handle); {
	case uint32(r) == cancelled:
		return "", nil
	case r != 0:
		return "", fmt.Errorf("the folder picker failed (error 0x%x)", r)
	}
	var item unsafe.Pointer
	if r := call(dialog, methodGetResult, uintptr(unsafe.Pointer(&item))); r != 0 || item == nil {
		return "", fmt.Errorf("the folder picker returned nothing (error 0x%x)", r)
	}
	defer call(item, methodRelease)
	var name *uint16
	if r := call(item, methodDisplayName, fileSystemPath, uintptr(unsafe.Pointer(&name))); r != 0 || name == nil {
		return "", fmt.Errorf("the chosen folder has no path (error 0x%x)", r)
	}
	defer ole32.NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(name)))
	return syscall.UTF16ToString((*[32768]uint16)(unsafe.Pointer(name))[:]), nil
}
