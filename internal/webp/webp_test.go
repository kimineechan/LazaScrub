package webp

import (
	"bytes"
	"encoding/binary"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func TestScanFindsEveryKind(t *testing.T) {
	found, err := Scan(samples.WebP(6))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct{ section, field string }{
		{"EXIF", "Camera make"}, {"GPS", "Position"}, {"XMP", "dc:creator"},
		{"WebP", "Chunk NOTE"}, {"After the content", "Hidden data"},
	} {
		if _, ok := meta.Lookup(found, w.section, w.field); !ok {
			t.Errorf("%s / %s is missing", w.section, w.field)
		}
	}
}

func TestCleanRemovesMetadata(t *testing.T) {
	for _, orientation := range []uint16{1, 6} {
		original := samples.WebP(orientation)
		cleaned, err := Clean(original)
		if err != nil {
			t.Fatal(err)
		}
		if int(binary.LittleEndian.Uint32(cleaned[4:]))+8 != len(cleaned) {
			t.Error("the file size in the header is wrong")
		}
		chunks, trailing, err := parse(cleaned)
		if err != nil || len(trailing) != 0 {
			t.Fatalf("the cleaned file is not well formed: %v", err)
		}
		flags := chunks[0].data[0]
		if chunks[0].kind != "VP8X" || flags&flagXMP != 0 || (flags&flagEXIF != 0) != (orientation == 6) {
			t.Errorf("orientation %d: header flags = %#x", orientation, flags)
		}
		a, _ := Fingerprint(original)
		b, _ := Fingerprint(cleaned)
		if !bytes.Equal(a, b) {
			t.Error("the picture data changed")
		}
		for _, secret := range samples.Secrets() {
			if bytes.Contains(cleaned, []byte(secret)) {
				t.Errorf("%q survived cleaning", secret)
			}
		}
		left, _ := Scan(cleaned)
		for _, f := range left {
			if !f.Kept {
				t.Errorf("%s / %s is still in the cleaned file", f.Section, f.Field)
			}
		}
		if _, rotated := meta.Lookup(left, "EXIF", "Orientation"); rotated != (orientation == 6) {
			t.Errorf("orientation %d: kept = %v", orientation, rotated)
		}
	}
}

func TestDamagedFilesDoNotCrash(t *testing.T) {
	whole := samples.WebP(6)
	for cut := 0; cut < len(whole); cut += 3 {
		Scan(whole[:cut])
		Clean(whole[:cut])
	}
}
