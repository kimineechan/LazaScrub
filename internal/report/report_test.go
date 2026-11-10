package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lazascrub/internal/samples"
	"lazascrub/internal/scrub"
)

var when = time.Date(2026, 11, 10, 14, 30, 0, 0, time.UTC)

func clean(t *testing.T) *scrub.Result {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(src, samples.JPEG(6), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := scrub.Clean(src, filepath.Join(dir, "photo-clean.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTextReports(t *testing.T) {
	result := clean(t)
	inspection := Text(result.Source, when)
	for _, want := range []string{
		"Metadata report", "on 2026-11-10 14:30", "photo.jpg", "JPEG image", "SHA-256 " + result.Source.SHA256,
		"37 fields found: 14 high risk, 16 medium, 7 low.", "HIGH RISK (14)", "MEDIUM RISK (16)", "LOW RISK (7)",
		"GPS / Position\r\n    51.476852, -0.000500",
	} {
		if !strings.Contains(inspection, want) {
			t.Errorf("the inspection report lacks %q", want)
		}
	}
	if strings.Index(inspection, "HIGH RISK") > strings.Index(inspection, "LOW RISK") {
		t.Error("high-risk findings must come first")
	}

	cleaning := CleanText(result, when)
	for _, want := range []string{
		"Cleaning report", "Original:", "Clean copy:", result.Output.SHA256, "Removed 35 of 37 fields. 2 kept on purpose.",
		"The picture data is byte-for-byte identical to the original.", "REMOVED (35)", "KEPT ON PURPOSE (2)",
		"[Low] EXIF / Orientation",
	} {
		if !strings.Contains(cleaning, want) {
			t.Errorf("the cleaning report lacks %q", want)
		}
	}
	if strings.Contains(cleaning, "STILL PRESENT") {
		t.Error("nothing should be listed as still present")
	}
}

func TestJSONReports(t *testing.T) {
	result := clean(t)
	var inspection struct {
		Tool     string
		File     struct{ Path, Type, SHA256 string }
		High     int
		Findings []struct{ Risk, Section, Field, Value string }
	}
	if err := json.Unmarshal(JSON(result.Source, when), &inspection); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(inspection.Tool, "LazaScrub ") || inspection.File.Type != "JPEG image" || inspection.High != 14 || len(inspection.Findings) != 37 {
		t.Errorf("inspection = %+v", inspection)
	}
	if inspection.Findings[0].Risk != "High" {
		t.Errorf("first finding = %+v", inspection.Findings[0])
	}

	var cleaning struct {
		ContentUnchanged    bool
		Removed, Kept       []struct{ Field string }
		StillPresent        []struct{ Field string }
		Original, CleanCopy struct{ SHA256 string }
	}
	if err := json.Unmarshal(CleanJSON(result, when), &cleaning); err != nil {
		t.Fatal(err)
	}
	if !cleaning.ContentUnchanged || len(cleaning.Removed) != 35 || len(cleaning.Kept) != 2 || cleaning.StillPresent == nil || len(cleaning.StillPresent) != 0 {
		t.Errorf("cleaning = %+v", cleaning)
	}
	if cleaning.Original.SHA256 == cleaning.CleanCopy.SHA256 {
		t.Error("the original and the copy have the same hash")
	}
}
