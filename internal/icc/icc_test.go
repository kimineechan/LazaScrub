package icc

import (
	"encoding/binary"
	"testing"

	"lazascrub/internal/meta"
)

// profile builds a minimal version 2 profile with a description and a copyright tag.
func profile() []byte {
	be := binary.BigEndian
	desc := append([]byte("desc\x00\x00\x00\x00\x00\x00\x00\x0FsRGB test tile\x00"), make([]byte, 5)...)
	text := []byte("text\x00\x00\x00\x00Example Works\x00\x00\x00")
	b := make([]byte, 128+4+2*12)
	copy(b[36:], "acsp")
	for i, v := range []uint16{2026, 3, 14, 9, 26, 53} {
		be.PutUint16(b[24+2*i:], v)
	}
	be.PutUint32(b[128:], 2)
	copy(b[132:], "desc")
	be.PutUint32(b[136:], uint32(len(b)))
	be.PutUint32(b[140:], uint32(len(desc)))
	copy(b[144:], "cprt")
	be.PutUint32(b[148:], uint32(len(b)+len(desc)))
	be.PutUint32(b[152:], uint32(len(text)))
	return append(append(b, desc...), text...)
}

func TestParseDescribesTheProfile(t *testing.T) {
	found := Parse(profile())
	for field, want := range map[string]string{
		"Profile name":      "sRGB test tile",
		"Profile copyright": "Example Works",
		"Profile created":   "2026-03-14 09:26:53",
	} {
		f, ok := meta.Lookup(found, "Colour profile", field)
		if !ok || f.Value != want {
			t.Errorf("%s = %q, want %q", field, f.Value, want)
		}
		if !f.Kept || f.Risk != meta.Low {
			t.Errorf("%s must be a low-risk finding that cleaning keeps", field)
		}
	}
}

func TestParseSurvivesDamage(t *testing.T) {
	whole := profile()
	for cut := 0; cut <= len(whole); cut++ {
		if found := Parse(whole[:cut]); len(found) == 0 {
			t.Errorf("a profile cut to %d bytes was not reported at all", cut)
		}
	}
}
