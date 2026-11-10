package png

import (
	"bytes"
	stdpng "image/png"
	"reflect"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func TestScanFindsEveryKind(t *testing.T) {
	found, err := Scan(samples.PNG())
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ section, field, value string }{
		{"PNG", "Resolution", "96 x 96 dpi"},
		{"Text", "Author", samples.Author},
		{"Text", "Software", samples.Software},
		{"Text", "Comment", "Exported for the client review"},
		{"Text", "parameters", "a lighthouse at dawn, 35mm photo Steps: 30, Seed: 1234567890"},
		{"XMP", "dc:creator", samples.Author},
		{"EXIF", "Camera model", samples.Model},
		{"GPS", "Position", "51.476852, -0.000500"},
		{"PNG", "Last modified", "2026-03-14 09:26:53 UTC"},
		{"PNG", "Private chunk prVW", "64 bytes"},
	}
	for _, w := range want {
		f, ok := meta.Lookup(found, w.section, w.field)
		if !ok {
			t.Errorf("%s / %s is missing", w.section, w.field)
		} else if f.Value != w.value {
			t.Errorf("%s / %s = %q, want %q", w.section, w.field, f.Value, w.value)
		}
	}
	if _, ok := meta.Lookup(found, "After the content", "Hidden data"); !ok {
		t.Error("data after the picture was not reported")
	}
}

func TestCleanRemovesMetadataAndKeepsThePicture(t *testing.T) {
	original := samples.PNG()
	cleaned, err := Clean(original)
	if err != nil {
		t.Fatal(err)
	}
	before, err := stdpng.Decode(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	after, err := stdpng.Decode(bytes.NewReader(cleaned))
	if err != nil {
		t.Fatalf("the picture no longer decodes: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("the cleaned picture differs from the original")
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
			t.Errorf("%s / %s = %q is still in the cleaned file", f.Section, f.Field, f.Value)
		}
	}
	if _, ok := meta.Lookup(left, "PNG", "Resolution"); !ok {
		t.Error("the resolution was lost")
	}
	again, _ := Clean(cleaned)
	if !bytes.Equal(again, cleaned) {
		t.Error("cleaning a clean file changed it")
	}
}

func TestDamagedFilesDoNotCrash(t *testing.T) {
	whole := samples.PNG()
	for cut := 0; cut < len(whole); cut += 7 {
		Scan(whole[:cut])
		Clean(whole[:cut])
	}
}
