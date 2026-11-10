package exif

import (
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func TestParseReadsEveryDirectory(t *testing.T) {
	info, err := Parse(samples.EXIF(6))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ section, field, value string }{
		{"EXIF", "Camera make", samples.Camera},
		{"EXIF", "Camera model", samples.Model},
		{"EXIF", "Orientation", "Rotated 90°"},
		{"EXIF", "Artist", samples.Author},
		{"EXIF", "Date taken", "2026-03-14 09:26:53"},
		{"EXIF", "Exposure time", "1/250 s"},
		{"EXIF", "Aperture", "f/1.8"},
		{"EXIF", "ISO speed", "200"},
		{"EXIF", "Focal length", "26 mm"},
		{"EXIF", "Camera serial number", samples.Serial},
		{"GPS", "Position", "51.476852, -0.000500"},
		{"GPS", "Altitude", "47 m above sea level"},
		{"GPS", "GPS version", "2.3.0.0"},
		{"Thumbnail", "Compression", "JPEG"},
	}
	for _, w := range want {
		f, ok := meta.Lookup(info.Findings, w.section, w.field)
		if !ok {
			t.Errorf("%s / %s is missing", w.section, w.field)
		} else if f.Value != w.value {
			t.Errorf("%s / %s = %q, want %q", w.section, w.field, f.Value, w.value)
		}
	}
	if info.Orientation != 6 {
		t.Errorf("orientation = %d, want 6", info.Orientation)
	}
	if len(info.Thumbnail) == 0 {
		t.Error("the preview image was not found")
	}
	if f, _ := meta.Lookup(info.Findings, "GPS", "Position"); f.Risk != meta.High {
		t.Error("a GPS position must be high risk")
	}
	if f, _ := meta.Lookup(info.Findings, "EXIF", "Orientation"); !f.Kept {
		t.Error("a rotated picture must keep its orientation")
	}
}

func TestOrientationBlockRoundTrips(t *testing.T) {
	for value := 1; value <= 8; value++ {
		info, err := Parse(Orientation(value))
		if err != nil {
			t.Fatal(err)
		}
		if info.Orientation != value {
			t.Errorf("orientation %d read back as %d", value, info.Orientation)
		}
		if len(info.Findings) != 1 {
			t.Errorf("orientation block holds %d fields, want 1", len(info.Findings))
		}
	}
}

// A damaged block must never crash the reader or make it read out of bounds.
func TestParseSurvivesDamage(t *testing.T) {
	whole := samples.EXIF(1)
	for cut := 0; cut < len(whole); cut += 3 {
		Parse(whole[:cut])
	}
	for at := 0; at < len(whole); at += 5 {
		broken := append([]byte(nil), whole...)
		broken[at] ^= 0xFF
		Parse(broken)
	}
}
