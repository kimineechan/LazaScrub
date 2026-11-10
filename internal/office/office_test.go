package office

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strings"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

type expect struct{ section, field, value string }

func check(t *testing.T, found []meta.Finding, want []expect) {
	t.Helper()
	for _, w := range want {
		f, ok := meta.Lookup(found, w.section, w.field)
		if !ok {
			t.Errorf("%s / %s is missing", w.section, w.field)
		} else if f.Value != w.value {
			t.Errorf("%s / %s = %q, want %q", w.section, w.field, f.Value, w.value)
		}
	}
}

// unpack returns every part of a package, and fails the test if the package
// is damaged, holds XML that does not parse, or refers to a part that is missing.
func unpack(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("the package is not a valid ZIP file: %v", err)
	}
	parts := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		parts[f.Name] = body
		if f.Modified.Year() > 1980 {
			t.Errorf("%s still records when it was saved", f.Name)
		}
	}
	for name, body := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		d := xml.NewDecoder(bytes.NewReader(body))
		for {
			if _, err := d.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Errorf("%s is no longer well-formed XML: %v", name, err)
				break
			}
		}
		if strings.HasSuffix(name, ".rels") {
			owner := path.Dir(path.Dir(name))
			for _, tag := range tags(body, "Relationship") {
				a := attrs(tag)
				if a["TargetMode"] == "External" {
					continue
				}
				if target := path.Join(owner, a["Target"]); parts[target] == nil {
					t.Errorf("%s points at %s, which is not in the package", name, target)
				}
			}
		}
		if name == "[Content_Types].xml" {
			for _, tag := range tags(body, "Override") {
				if part := strings.TrimPrefix(attrs(tag)["PartName"], "/"); parts[part] == nil {
					t.Errorf("the content types list %s, which is not in the package", part)
				}
			}
		}
	}
	return parts
}

func checkClean(t *testing.T, original []byte, kept []string) map[string][]byte {
	t.Helper()
	cleaned, err := Clean(original)
	if err != nil {
		t.Fatal(err)
	}
	parts := unpack(t, cleaned)
	var all bytes.Buffer
	for name, body := range parts {
		all.WriteString(name + "\n")
		all.Write(body)
	}
	secrets := append(samples.Secrets(), samples.Reviewer, samples.Company, "alex.example", "Budget.dotm", "sam.reviewer",
		"Jordan Lead", "Northlight plc", "00A31F5C", "00B7204D", "2026-03-1", "thumbnail", "attachedTemplate",
		"/printerSettings/", "EXAMPLE-PRINT01", "absPath", "Microsoft Office", "Microsoft Excel")
	for _, secret := range secrets {
		if bytes.Contains(all.Bytes(), []byte(secret)) {
			t.Errorf("%q survived cleaning", secret)
		}
	}
	for _, text := range kept {
		if !bytes.Contains(all.Bytes(), []byte(text)) {
			t.Errorf("%q was lost", text)
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
	before, _ := Fingerprint(original)
	after, _ := Fingerprint(cleaned)
	if !bytes.Equal(before, after) {
		t.Error("the document content changed")
	}
	again, err := Clean(cleaned)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range unpack(t, again) {
		if !bytes.Equal(body, parts[name]) {
			t.Errorf("cleaning a clean file changed %s", name)
		}
	}
	return parts
}

func TestWordDocument(t *testing.T) {
	original := samples.DOCX()
	if Kind(original) != "Word document" {
		t.Errorf("kind = %q", Kind(original))
	}
	found, err := Scan(original)
	if err != nil {
		t.Fatal(err)
	}
	check(t, found, []expect{
		{"Document properties", "Title", "Budget summary"},
		{"Document properties", "Author", samples.Author},
		{"Document properties", "Last modified by", samples.Reviewer},
		{"Document properties", "Revision number", "7"},
		{"Document properties", "Created", "2026-03-14T09:26:00Z"},
		{"Document properties", "Last printed", "2026-03-15T17:40:00Z"},
		{"Document properties", "Template", "Budget.dotm"},
		{"Document properties", "Editing time (minutes)", "95"},
		{"Document properties", "Application", "Microsoft Office Word"},
		{"Document properties", "Manager", "Jordan Lead"},
		{"Document properties", "Company", samples.Company},
		{"Custom properties", "Client", "Northlight plc"},
		{"Comments", "Comment author", "Sam Reviewer (1 comment)"},
		{"Tracked changes", "Change author", "Sam Reviewer (2 changes)"},
		{"Tracked changes", "Deleted text", "1 deletion still stored"},
		{"Revision IDs", "Editing sessions", "2 session IDs"},
		{"People", "Account", "Sam Reviewer (" + samples.Account + ")"},
		{"Document settings", "Attached template", samples.TemplatePath},
		{"Picture word/media/image1.jpeg > GPS", "Position", "51.476852, -0.000500"},
		{"Package", "Part timestamps", "Newest 2026-03-15 18:02:11 UTC"},
	})
	if _, ok := meta.Lookup(found, "Thumbnail", "Preview image"); !ok {
		t.Error("the thumbnail was not reported")
	}
	if f, _ := meta.Lookup(found, "Document settings", "Attached template"); f.Risk != meta.High {
		t.Error("a template path with a user name must be high risk")
	}
	parts := checkClean(t, original, []string{
		"Budget summary for the board", "Check this figure before it goes out", "after the correction", samples.DeletedText,
	})
	if !bytes.Contains(parts["word/comments.xml"], []byte(`w:author="Author"`)) {
		t.Error("the comment author was not replaced")
	}
}

func TestExcelWorkbook(t *testing.T) {
	original := samples.XLSX()
	found, err := Scan(original)
	if err != nil {
		t.Fatal(err)
	}
	check(t, found, []expect{
		{"Document properties", "Author", samples.Author},
		{"Workbook", "Saved in folder", samples.SavedIn},
		{"Printer settings", "Printer", samples.Printer},
		{"Hidden content", "Hidden sheet", "Salaries"},
		{"Comments", "Comment author", "Sam Reviewer (1 comment)"},
	})
	parts := checkClean(t, original, []string{"Budget summary for the board", "Salary bands", "Check this figure before it goes out"})
	if sheet := parts["xl/worksheets/sheet1.xml"]; !bytes.Contains(sheet, []byte(`<pageSetup paperSize="9" orientation="portrait"/>`)) {
		t.Errorf("the page setup was not detached from the printer settings: %s", sheet)
	}
	if !bytes.Contains(parts["xl/comments1.xml"], []byte("<author>Author</author>")) {
		t.Error("the comment author was not replaced")
	}
}

func TestPowerPointPresentation(t *testing.T) {
	original := samples.PPTX()
	found, err := Scan(original)
	if err != nil {
		t.Fatal(err)
	}
	check(t, found, []expect{
		{"Document properties", "Application", "Microsoft Office PowerPoint"},
		{"Comments", "Comment author", "Sam Reviewer (1 comment)"},
		{"Hidden content", "Hidden slide", "slide2.xml"},
		{"Picture ppt/media/image1.png > Text", "Author", samples.Author},
		{"Picture ppt/media/image1.png > GPS", "Position", "51.476852, -0.000500"},
	})
	parts := checkClean(t, original, []string{"Budget summary for the board", "Salary bands"})
	if !bytes.Contains(parts["ppt/commentAuthors.xml"], []byte(`name="Author" initials="A"`)) {
		t.Errorf("the comment author was not replaced: %s", parts["ppt/commentAuthors.xml"])
	}
}

func TestOtherZipFilesAreRejected(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	part, _ := w.Create("notes.txt")
	part.Write([]byte("not an office document"))
	w.Close()
	if Is(buf.Bytes()) {
		t.Error("a plain ZIP file was taken for an Office document")
	}
	if _, err := Clean(buf.Bytes()); err == nil {
		t.Error("a plain ZIP file was cleaned as an Office document")
	}
}

func TestDamagedFilesDoNotCrash(t *testing.T) {
	whole := samples.DOCX()
	for cut := 0; cut < len(whole); cut += 131 {
		Scan(whole[:cut])
		Clean(whole[:cut])
	}
}
