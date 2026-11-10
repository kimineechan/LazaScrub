package scrub

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func put(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectDetectsTheTypeFromTheContent(t *testing.T) {
	dir := t.TempDir()
	for name, want := range map[string]string{
		"a.bin": "JPEG image", "b.bin": "PNG image", "c.bin": "GIF image", "d.bin": "WebP image",
		"e.bin": "PDF document", "f.bin": "Word document", "g.bin": "Excel workbook", "h.bin": "PowerPoint presentation",
	} {
		data := map[string][]byte{
			"a.bin": samples.JPEG(1), "b.bin": samples.PNG(), "c.bin": samples.GIF(), "d.bin": samples.WebP(1),
			"e.bin": samples.PDF(), "f.bin": samples.DOCX(), "g.bin": samples.XLSX(), "h.bin": samples.PPTX(),
		}[name]
		report, err := Inspect(put(t, dir, name, data))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if report.Format != want {
			t.Errorf("%s: format = %q, want %q", name, report.Format, want)
		}
		if report.Size != int64(len(data)) || len(report.SHA256) != 64 {
			t.Errorf("%s: size %d, hash %q", name, report.Size, report.SHA256)
		}
		for i := 1; i < len(report.Findings); i++ {
			if report.Findings[i].Risk > report.Findings[i-1].Risk {
				t.Errorf("%s: findings are not ordered from high to low risk", name)
				break
			}
		}
	}
	if _, err := Inspect(put(t, dir, "notes.txt", []byte("plain text"))); err != ErrUnsupported {
		t.Errorf("a text file gave %v, want ErrUnsupported", err)
	}
	if _, err := Inspect(dir); err == nil {
		t.Error("a folder was inspected as a file")
	}
}

func TestCleanWritesACheckedCopyAndLeavesTheOriginalAlone(t *testing.T) {
	dir := t.TempDir()
	original := samples.JPEG(6)
	src := put(t, dir, "photo.jpg", original)
	dst := filepath.Join(dir, "photo-clean.jpg")
	result, err := Clean(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Same {
		t.Error("the picture data was reported as changed")
	}
	if len(result.Left) != 0 {
		t.Errorf("fields left behind: %+v", result.Left)
	}
	if len(result.Kept) != 2 || len(result.Removed) != len(result.Source.Findings)-2 {
		t.Errorf("removed %d, kept %d of %d", len(result.Removed), len(result.Kept), len(result.Source.Findings))
	}
	if _, ok := meta.Lookup(result.Kept, "EXIF", "Orientation"); !ok {
		t.Error("the orientation should be listed as kept")
	}
	if _, ok := meta.Lookup(result.Removed, "GPS", "Position"); !ok {
		t.Error("the GPS position should be listed as removed")
	}
	if want := "Removed 35 of 37 fields. 2 kept on purpose."; result.Summary() != want {
		t.Errorf("summary = %q, want %q", result.Summary(), want)
	}
	if after, _ := os.ReadFile(src); !bytes.Equal(after, original) {
		t.Error("the original file was modified")
	}
	cleaned, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if result.Output.Size != int64(len(cleaned)) || result.Output.SHA256 != digest(cleaned) {
		t.Error("the report does not describe the file that was written")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("%d files in the folder, want the original and the copy only", len(entries))
	}
}

func TestCleanRefusesToOverwriteTheOriginal(t *testing.T) {
	dir := t.TempDir()
	original := samples.PNG()
	src := put(t, dir, "picture.png", original)
	for _, dst := range []string{src, filepath.Join(dir, ".", "picture.png"), filepath.Join(dir, "PICTURE.PNG")} {
		if _, err := os.Stat(dst); err != nil {
			continue // the file system tells names apart by case
		}
		if _, err := Clean(src, dst); err == nil {
			t.Errorf("Clean(%q) overwrote the original", dst)
		}
	}
	if after, _ := os.ReadFile(src); !bytes.Equal(after, original) {
		t.Error("the original file was modified")
	}
}

func TestCleanLeavesNoFileBehindWhenItFails(t *testing.T) {
	dir := t.TempDir()
	locked := bytes.Replace(samples.PDF(), []byte("/Info 10 0 R /Prev"), []byte("/Info 10 0 R /Encrypt << /Filter /Standard >> /Prev"), 1)
	src := put(t, dir, "locked.pdf", locked)
	if _, err := Clean(src, filepath.Join(dir, "locked-clean.pdf")); err == nil {
		t.Fatal("a password-protected PDF was cleaned")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d files in the folder after a failed clean, want 1", len(entries))
	}
}

func TestFolderIsSurveyedAndCleaned(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "Shared")
	put(t, folder, "photo.jpg", samples.JPEG(1))
	put(t, folder, "Report.PDF", samples.PDF())
	put(t, folder, "notes.txt", []byte("not a supported type"))
	put(t, folder, filepath.Join("older", "budget.xlsx"), samples.XLSX())

	var names []string
	if err := Survey(folder, false, func(done, total int, item Item) {
		if item.Err != nil || item.Report == nil || total != 2 {
			t.Errorf("%s: %v (total %d)", item.Name, item.Err, total)
		}
		names = append(names, item.Name)
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "photo.jpg,Report.PDF" {
		t.Errorf("surveyed %v", names)
	}

	target := FreshFolder(folder)
	if target != folder+"-clean" {
		t.Errorf("target = %q", target)
	}
	cleaned := 0
	if err := CleanFolder(folder, target, true, func(done, total int, item Item) {
		if item.Err != nil || !item.Result.Same || len(item.Result.Left) != 0 {
			t.Errorf("%s: %v", item.Name, item.Err)
		}
		cleaned++
	}); err != nil {
		t.Fatal(err)
	}
	if cleaned != 3 {
		t.Errorf("cleaned %d files, want 3", cleaned)
	}
	for _, name := range []string{"photo.jpg", "Report.PDF", filepath.Join("older", "budget.xlsx")} {
		if _, err := os.Stat(filepath.Join(target, name)); err != nil {
			t.Errorf("%s is missing from the clean folder", name)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "notes.txt")); err == nil {
		t.Error("a file of an unsupported type was copied")
	}
	if next := FreshFolder(folder); next != folder+"-clean-2" {
		t.Errorf("second target = %q", next)
	}
	if err := CleanFolder(folder, target, true, func(int, int, Item) {}); err == nil {
		t.Error("an existing folder was reused")
	}
}
