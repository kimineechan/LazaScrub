// verify runs a complete round trip on sample files of every supported type:
// write, inspect, clean, inspect again. The release build runs it, so a
// release is never published with a handler that leaves metadata behind.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"lazascrub/internal/samples"
	"lazascrub/internal/scrub"
)

var files = []struct {
	name string
	data func() []byte
}{
	{"photo.jpg", func() []byte { return samples.JPEG(6) }},
	{"picture.png", samples.PNG},
	{"animation.gif", samples.GIF},
	{"picture.webp", func() []byte { return samples.WebP(6) }},
	{"report.pdf", samples.PDF},
	{"report-compressed.pdf", samples.CompressedPDF},
	{"report.docx", samples.DOCX},
	{"budget.xlsx", samples.XLSX},
	{"slides.pptx", samples.PPTX},
}

func main() {
	keep := flag.String("samples", "", "write the sample files to this folder and keep them")
	flag.Parse()
	if err := check(*keep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Release verification passed: %d sample files inspected, cleaned and checked.\n", len(files))
}

func check(keep string) error {
	dir := keep
	if dir == "" {
		temp, err := os.MkdirTemp("", "lazascrub-verify-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		dir = temp
	}
	if err := os.MkdirAll(filepath.Join(dir, "clean"), 0700); err != nil {
		return err
	}
	for _, f := range files {
		original := f.data()
		src, dst := filepath.Join(dir, f.name), filepath.Join(dir, "clean", f.name)
		if err := os.WriteFile(src, original, 0600); err != nil {
			return err
		}
		os.Remove(dst)
		result, err := scrub.Clean(src, dst)
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		switch {
		case len(result.Removed) == 0:
			return fmt.Errorf("%s: nothing was removed from a file that is full of metadata", f.name)
		case len(result.Left) > 0:
			return fmt.Errorf("%s: %s / %s was not removed", f.name, result.Left[0].Section, result.Left[0].Field)
		case !result.Same:
			return fmt.Errorf("%s: the content of the cleaned copy differs from the original", f.name)
		}
		for _, finding := range result.Output.Findings {
			if !finding.Kept {
				return fmt.Errorf("%s: the cleaned copy still reports %s / %s", f.name, finding.Section, finding.Field)
			}
		}
		after, err := os.ReadFile(src)
		if err != nil || !bytes.Equal(after, original) {
			return fmt.Errorf("%s: the original file changed", f.name)
		}
		fmt.Printf("%-24s %s\n", f.name, result.Summary())
	}
	return nil
}
