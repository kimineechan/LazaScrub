package jpeg

import (
	"bytes"
	"image"
	stdjpeg "image/jpeg"
	"reflect"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the picture no longer decodes: %v", err)
	}
	return img
}

func TestScanFindsEveryKind(t *testing.T) {
	found, err := Scan(samples.JPEG(6))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct{ section, field string }{
		{"JFIF header", "Resolution"}, {"EXIF", "Camera make"}, {"GPS", "Position"},
		{"Thumbnail", "Preview image"}, {"XMP", "dc:creator"}, {"XMP", "xmpMM:History [2]"},
		{"IPTC", "By-line (author)"}, {"IPTC", "City"}, {"Comment", "Comment"},
		{"After the content", "Hidden data"},
	} {
		if _, ok := meta.Lookup(found, w.section, w.field); !ok {
			t.Errorf("%s / %s is missing", w.section, w.field)
		}
	}
	if f, _ := meta.Lookup(found, "After the content", "Hidden data"); f.Risk != meta.High {
		t.Error("data after the picture must be high risk")
	}
}

func TestCleanRemovesMetadataAndKeepsThePicture(t *testing.T) {
	for _, orientation := range []uint16{1, 6} {
		original := samples.JPEG(orientation)
		cleaned, err := Clean(original)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decode(t, original), decode(t, cleaned)) {
			t.Error("the cleaned picture differs from the original")
		}
		before, _ := Fingerprint(original)
		after, _ := Fingerprint(cleaned)
		if !bytes.Equal(before, after) {
			t.Error("the picture data changed")
		}
		for _, secret := range samples.Secrets() {
			if bytes.Contains(cleaned, []byte(secret)) {
				t.Errorf("%q survived cleaning", secret)
			}
		}
		left, err := Scan(cleaned)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range left {
			if !f.Kept {
				t.Errorf("%s / %s = %q is still in the cleaned file", f.Section, f.Field, f.Value)
			}
		}
		_, rotated := meta.Lookup(left, "EXIF", "Orientation")
		if rotated != (orientation == 6) {
			t.Errorf("orientation %d: kept = %v", orientation, rotated)
		}
		again, _ := Clean(cleaned)
		if !bytes.Equal(again, cleaned) {
			t.Error("cleaning a clean file changed it")
		}
	}
}

func TestDamagedFilesDoNotCrash(t *testing.T) {
	whole := samples.JPEG(6)
	for cut := 0; cut < len(whole); cut += 7 {
		Scan(whole[:cut])
		Clean(whole[:cut])
	}
}

// A file cut short in the picture data must come back with that data intact.
func TestCleanKeepsTruncatedPictureData(t *testing.T) {
	whole := samples.JPEG(1)
	cut := whole[:len(whole)-len(samples.Hidden)-200]
	cleaned, err := Clean(cut)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(cleaned, cut[len(cut)-64:]) {
		t.Error("the end of the picture data was lost")
	}
}
