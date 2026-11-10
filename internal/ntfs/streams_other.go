//go:build !windows

// Package ntfs reads the alternate data streams that Windows can attach to a
// file. Other systems have none, so this build reports nothing.
package ntfs

// Stream is one named data stream beside a file's main content.
type Stream struct {
	Name string
	Size int64
}

func Streams(path string) []Stream { return nil }

func Read(path, stream string, limit int) []byte { return nil }
