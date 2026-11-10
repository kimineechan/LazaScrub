package iptc

import (
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func TestParseReadsCaptionData(t *testing.T) {
	found := Parse(samples.IPTC())
	want := map[string]string{
		"By-line (author)": samples.Author,
		"City":             "Greenwich",
		"Country":          "United Kingdom",
		"Copyright notice": "(c) 2026 Alex Example",
		"Caption":          "Sunrise over the observatory",
	}
	for field, value := range want {
		if f, ok := meta.Lookup(found, "IPTC", field); !ok || f.Value != value {
			t.Errorf("%s = %q, want %q", field, f.Value, value)
		}
	}
	if len(found) != len(want) {
		t.Errorf("found %d datasets, want %d", len(found), len(want))
	}
}

func TestParseSurvivesDamage(t *testing.T) {
	whole := samples.IPTC()
	for cut := 0; cut < len(whole); cut++ {
		Parse(whole[:cut])
	}
}
