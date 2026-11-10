//go:build windows

package scrub

import (
	"os"
	"path/filepath"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

// Windows records where a download came from in a hidden stream beside the
// file. Inspecting must report it, and a cleaned copy must not carry it.
func TestDownloadRecordIsReportedAndNotCopied(t *testing.T) {
	dir := t.TempDir()
	src := put(t, dir, "photo.jpg", samples.JPEG(1))
	record := "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://example.com/gallery\r\nHostUrl=https://example.com/files/photo.jpg\r\n"
	if err := os.WriteFile(src+":Zone.Identifier", []byte(record), 0600); err != nil {
		t.Skip("this file system has no alternate data streams:", err)
	}
	report, err := Inspect(src)
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{
		"Came from":        "The internet",
		"Downloaded from":  "https://example.com/files/photo.jpg",
		"Linked from page": "https://example.com/gallery",
	} {
		if f, ok := meta.Lookup(report.Findings, "Download record", field); !ok || f.Value != want {
			t.Errorf("%s = %q, want %q", field, f.Value, want)
		}
	}
	result, err := Clean(src, filepath.Join(dir, "photo-clean.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.Lookup(result.Output.Findings, "Download record", "Downloaded from"); ok {
		t.Error("the cleaned copy still has a download record")
	}
	if _, ok := meta.Lookup(result.Removed, "Download record", "Downloaded from"); !ok {
		t.Error("the download record is not listed as removed")
	}
}
