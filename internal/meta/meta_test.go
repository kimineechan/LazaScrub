package meta

import "testing"

func TestRiskIsJudgedFromTheNameAndTheValue(t *testing.T) {
	for _, c := range []struct {
		name, value string
		want        Risk
	}{
		{"GPSLatitude", "51.5", High},
		{"dc:creator", "Alex Example", High},
		{"SerialNumber", "12345", High},
		{"CreateDate", "2026-03-14", Medium},
		{"Software", "Northlight", Medium},
		{"ColorMode", "3", Low},
		{"Template", `C:\Users\alex\Templates\Budget.dotm`, High},
		{"Source", "file:///home/alex/scan.png", High},
		{"Note", "send to alex@example.com", High},
		{"Share", `\\server\finance\budget.xlsx`, High},
		{"ColorMode", "RGB", Low},
	} {
		if got := Rate(c.name, c.value); got != c.want {
			t.Errorf("Rate(%q, %q) = %v, want %v", c.name, c.value, got, c.want)
		}
	}
}

func TestTextDecodesTheEncodingsFilesUse(t *testing.T) {
	for _, c := range []struct {
		in   []byte
		want string
	}{
		{[]byte("plain"), "plain"},
		{[]byte("Caf\xC3\xA9"), "Café"},
		{[]byte("Caf\xE9"), "Café"},
		{[]byte("\xFE\xFF\x00H\x00i"), "Hi"},
		{[]byte("\xFF\xFEH\x00i\x00"), "Hi"},
		{[]byte("\xEF\xBB\xBFbom"), "bom"},
		{[]byte("two\r\nlines\tand  spaces\x00"), "two lines and spaces"},
	} {
		if got := Text(c.in); got != c.want {
			t.Errorf("Text(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSizesAndClipping(t *testing.T) {
	for n, want := range map[int64]string{0: "0 bytes", 743: "743 bytes", 1536: "1.5 KB", 5 << 20: "5.0 MB", 3 << 30: "3.00 GB"} {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
	if got := Clip("abcdefghij", 8); got != "abcde..." {
		t.Errorf("Clip = %q", got)
	}
	if got := Clip("short", 8); got != "short" {
		t.Errorf("Clip = %q", got)
	}
}

func TestSniffNamesCommonFileTypes(t *testing.T) {
	for in, want := range map[string]string{
		"PK\x03\x04rest":             "a ZIP archive",
		"\xFF\xD8\xFF\xE0":           "a JPEG image",
		"%PDF-1.7":                   "a PDF document",
		"\x00\x00\x00\x18ftypmp42":   "an MP4 or QuickTime video",
		"\x00\x00\x00\x00":           "padding (zero bytes)",
		"just some unrelated bytes.": "unrecognised data",
	} {
		if got := Sniff([]byte(in)); got != want {
			t.Errorf("Sniff(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListHelpers(t *testing.T) {
	var list List
	list.Add("EXIF", "Artist", "Alex", High, "")
	list.Keep("EXIF", "Orientation", "Rotated 90°", Low, "why")
	list.Nest("Picture in object 9", []Finding{{Section: "GPS", Field: "Position", Value: "1, 2", Risk: High}})
	list.Hidden(nil)
	list.Hidden([]byte("PK\x03\x04"))
	if len(list) != 4 {
		t.Fatalf("list holds %d findings, want 4", len(list))
	}
	if !list[1].Kept || list[0].Kept {
		t.Error("only the second finding is kept")
	}
	if list[2].Section != "Picture in object 9 > GPS" {
		t.Errorf("nested section = %q", list[2].Section)
	}
	if f, ok := Lookup(list, "After the content", "Hidden data"); !ok || f.Value != "4 bytes, looks like a ZIP archive" {
		t.Errorf("hidden data = %+v", f)
	}
	if list[0].Key() == list[1].Key() {
		t.Error("different findings share a key")
	}
}
