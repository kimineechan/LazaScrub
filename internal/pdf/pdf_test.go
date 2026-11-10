package pdf

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func secrets() []string {
	return append(samples.Secrets(), samples.DraftTitle, samples.DraftText, samples.Reviewer,
		"Example Holdings", "Northlight Writer", "Exif", "8F1C2B7A", "/Metadata", "/Info", "/PieceInfo", "/Thumb")
}

func TestScanReportsPropertiesAndHistory(t *testing.T) {
	found, err := Scan(samples.PDF())
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ section, field, value string }{
		{"Document information", "Title", samples.FinalTitle},
		{"Document information", "Author", samples.Author},
		{"Document information", "Created with", "Northlight Writer 12"},
		{"Document information", "PDF producer", "Northlight PDF Library 3.4"},
		{"Document information", "Created", "2026-03-14 09:26:53 +01:00"},
		{"Document information", "Modified", "2026-03-15 18:02:11 UTC"},
		{"Document information", "Company", "Example Holdings Ltd"},
		{"XMP", "dc:creator", samples.Author},
		{"File structure", "Document ID", "8f1c2b7a90e44d3ba1c5d6e7f8091a2b / 3d4e5f60718293a4b5c6d7e8f9a0b1c2"},
		{"Earlier versions", "Saved revisions", "1 earlier version of the document still in the file"},
		{"Earlier version > Document information", "Title", samples.DraftTitle},
		{"Comments", "Comment author", "Sam Reviewer (1 comment)"},
		{"Thumbnails", "Page thumbnails", "1 page"},
		{"Private app data", "Editing data", "1 block"},
		{"Picture in object 9 > GPS", "Position", "51.476852, -0.000500"},
		{"Picture in object 9 > EXIF", "Camera serial number", samples.Serial},
	}
	for _, w := range want {
		f, ok := meta.Lookup(found, w.section, w.field)
		if !ok {
			t.Errorf("%s / %s is missing", w.section, w.field)
		} else if f.Value != w.value {
			t.Errorf("%s / %s = %q, want %q", w.section, w.field, f.Value, w.value)
		}
	}
	if f, ok := meta.Lookup(found, "Earlier versions", "Replaced objects"); !ok || !strings.HasPrefix(f.Value, "2 objects, ") {
		t.Errorf("replaced objects = %q", f.Value)
	}
	if _, ok := meta.Lookup(found, "Earlier version > Document information", "Author"); ok {
		t.Error("a value that did not change was reported as an earlier version")
	}
}

func checkClean(t *testing.T, original []byte) []byte {
	t.Helper()
	cleaned, err := Clean(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets() {
		if bytes.Contains(cleaned, []byte(secret)) {
			t.Errorf("%q survived cleaning", secret)
		}
	}
	if !bytes.Contains(cleaned, []byte(samples.FinalText)) {
		t.Error("the page text was lost")
	}
	if left, _ := Scan(cleaned); len(left) != 0 {
		t.Errorf("%d fields are still in the cleaned file: %+v", len(left), left)
	}
	if pages, err := Pages(cleaned); err != nil || pages != 1 {
		t.Errorf("pages = %d, %v", pages, err)
	}
	before, _ := Fingerprint(original)
	after, _ := Fingerprint(cleaned)
	if !bytes.Equal(before, after) {
		t.Error("the document content changed")
	}
	again, err := Clean(cleaned)
	if err != nil || !bytes.Equal(again, cleaned) {
		t.Errorf("cleaning a clean file changed it (%v)", err)
	}
	return cleaned
}

func TestCleanDropsMetadataAndEarlierVersions(t *testing.T) {
	cleaned := checkClean(t, samples.PDF())
	if !bytes.Contains(cleaned, []byte("Check this figure before it goes out")) {
		t.Error("the text of the review comment was lost")
	}
}

func TestCompressedLayout(t *testing.T) {
	original := samples.CompressedPDF()
	found, err := Scan(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct{ section, field, value string }{
		{"Document information", "Title", samples.FinalTitle},
		{"Document information", "Company", "Example Holdings Ltd"},
		{"XMP", "photoshop:City", "Greenwich"},
		{"Picture in object 9 > GPS", "Position", "51.476852, -0.000500"},
	} {
		if f, ok := meta.Lookup(found, w.section, w.field); !ok || f.Value != w.value {
			t.Errorf("%s / %s = %q, want %q", w.section, w.field, f.Value, w.value)
		}
	}
	if _, ok := meta.Lookup(found, "Earlier versions", "Replaced objects"); ok {
		t.Error("a file saved once reported replaced objects")
	}
	checkClean(t, original)
}

// A file whose cross-reference table is unusable is read by scanning for its objects.
func TestDamagedTableIsRebuilt(t *testing.T) {
	broken := samples.PDF()
	i := bytes.LastIndex(broken, []byte("startxref"))
	copy(broken[i+len("startxref\n"):], "0000")
	found, err := Scan(broken)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := meta.Lookup(found, "Document information", "Title"); !ok || f.Value != samples.FinalTitle {
		t.Errorf("title = %q", f.Value)
	}
	if _, ok := meta.Lookup(found, "Earlier version > Document information", "Title"); !ok {
		t.Error("the earlier title was not found")
	}
	checkClean(t, broken)
}

func TestEncryptedDocumentIsRefused(t *testing.T) {
	locked := bytes.Replace(samples.PDF(), []byte("/Info 10 0 R /Prev"), []byte("/Info 10 0 R /Encrypt << /Filter /Standard /V 1 /R 2 >> /Prev"), 1)
	if _, err := Clean(locked); !errors.Is(err, ErrEncrypted) {
		t.Errorf("Clean = %v, want ErrEncrypted", err)
	}
	found, err := Scan(locked)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.Lookup(found, "Security", "Encryption"); !ok {
		t.Error("encryption was not reported")
	}
	if f, _ := meta.Lookup(found, "Document information", "Author"); f.Value != "(encrypted)" {
		t.Errorf("author = %q, want a placeholder", f.Value)
	}
}

func TestDamagedFilesDoNotCrash(t *testing.T) {
	for _, whole := range [][]byte{samples.PDF(), samples.CompressedPDF()} {
		for cut := 0; cut < len(whole); cut += 97 {
			Scan(whole[:cut])
			Clean(whole[:cut])
		}
		for at := 0; at < len(whole); at += 211 {
			broken := append([]byte(nil), whole...)
			broken[at] ^= 0x5A
			Scan(broken)
			Clean(broken)
		}
	}
}

func TestParserReadsEveryValueType(t *testing.T) {
	source := `<< /Name /A#20B /Int -12 /Real 3.5 /Ref 7 0 R /Text (a\(b\)\101\n) /Hex <48 65 6C6C 6F> /List [1 2 0 R true null] /Nested << /K (v) >> >>`
	p := &parser{src: []byte(source)}
	v, err := p.object()
	if err != nil {
		t.Fatal(err)
	}
	d := v.(*Dict)
	if d.Get("Name") != Name("A B") || d.Get("Int") != int64(-12) || d.Get("Real") != 3.5 || d.Get("Ref") != (Ref{7, 0}) {
		t.Errorf("scalars: %+v", d.entries)
	}
	if got := string(d.Get("Text").(String)); got != "a(b)A\n" {
		t.Errorf("literal string = %q", got)
	}
	if got := string(d.Get("Hex").(String)); got != "Hello" {
		t.Errorf("hex string = %q", got)
	}
	items := d.Get("List").(Array)
	if len(items) != 4 || items[0] != int64(1) || items[1] != (Ref{2, 0}) || items[2] != true || items[3] != nil {
		t.Errorf("array = %+v", items)
	}
	if got := string(d.Get("Nested").(*Dict).Get("K").(String)); got != "v" {
		t.Errorf("nested = %q", got)
	}
}

func TestTextAndDates(t *testing.T) {
	if got := text(String("\xFE\xFF\x00C\x00a\x00f\x00\xE9")); got != "Café" {
		t.Errorf("UTF-16 text = %q", got)
	}
	if got := text(String("Caf\xE9 \x8Dquoted\x8E")); got != "Café “quoted”" {
		t.Errorf("document encoding = %q", got)
	}
	for in, want := range map[string]string{
		"D:20260314092653+01'00'": "2026-03-14 09:26:53 +01:00",
		"D:20260314092653Z":       "2026-03-14 09:26:53 UTC",
		"D:2026":                  "2026",
		"D:20260314":              "2026-03-14",
		"14 March 2026":           "14 March 2026",
	} {
		if got := date(in); got != want {
			t.Errorf("date(%q) = %q, want %q", in, got, want)
		}
	}
}
