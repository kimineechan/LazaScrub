//go:build windows

// Package ntfs reads the alternate data streams that Windows can attach to a
// file, such as the record of where a download came from.
package ntfs

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Stream is one named data stream beside a file's main content.
type Stream struct {
	Name string
	Size int64
}

var (
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	findFirst   = kernel32.NewProc("FindFirstStreamW")
	findNext    = kernel32.NewProc("FindNextStreamW")
	findClose   = kernel32.NewProc("FindClose")
	invalidFind = ^uintptr(0)
)

// findData mirrors WIN32_FIND_STREAM_DATA.
type findData struct {
	Size int64
	Name [296]uint16
}

// Streams lists the named streams of a file. The main, unnamed stream is left out.
func Streams(path string) []Stream {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	var data findData
	handle, _, _ := findFirst.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&data)), 0)
	if handle == invalidFind {
		return nil
	}
	defer findClose.Call(handle)
	var out []Stream
	for {
		// Names look like ":Zone.Identifier:$DATA"; the main stream is "::$DATA".
		stream := strings.TrimSuffix(strings.TrimPrefix(syscall.UTF16ToString(data.Name[:]), ":"), ":$DATA")
		if stream != "" {
			out = append(out, Stream{Name: stream, Size: data.Size})
		}
		if ok, _, _ := findNext.Call(handle, uintptr(unsafe.Pointer(&data))); ok == 0 {
			return out
		}
	}
}

// Read returns up to limit bytes of a named stream.
func Read(path, stream string, limit int) []byte {
	f, err := os.Open(path + ":" + stream)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, _ := f.Read(buf)
	return buf[:n]
}
