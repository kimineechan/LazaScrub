// Package scrub inspects and cleans whole files. It picks the handler for a
// file's format, and after cleaning it scans the result again to show what
// is gone and what is left.
package scrub

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lazascrub/internal/gif"
	"lazascrub/internal/jpeg"
	"lazascrub/internal/meta"
	"lazascrub/internal/ntfs"
	"lazascrub/internal/office"
	"lazascrub/internal/pdf"
	"lazascrub/internal/png"
	"lazascrub/internal/webp"
)

// MaxSize is the largest file that is read.
const MaxSize = 1 << 30

// Extensions lists the file types that are picked up when a folder is cleaned.
var Extensions = []string{
	".jpg", ".jpeg", ".jpe", ".jfif", ".png", ".gif", ".webp", ".pdf",
	".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm",
	".pptx", ".pptm", ".potx", ".potm", ".ppsx", ".ppsm",
}

// Supported reports whether a file name has one of the supported extensions.
func Supported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range Extensions {
		if e == ext {
			return true
		}
	}
	return false
}

// handler is the set of operations one file format provides.
type handler struct {
	name        func([]byte) string
	is          func([]byte) bool
	scan        func([]byte) ([]meta.Finding, error)
	clean       func([]byte) ([]byte, error)
	fingerprint func([]byte) ([]byte, error)
	proof       string // what an unchanged fingerprint shows
}

func named(s string) func([]byte) string { return func([]byte) string { return s } }

const pictureProof = "The picture data is byte-for-byte identical to the original."

var handlers = []handler{
	{named("JPEG image"), jpeg.Is, jpeg.Scan, jpeg.Clean, jpeg.Fingerprint, pictureProof},
	{named("PNG image"), png.Is, png.Scan, png.Clean, png.Fingerprint, pictureProof},
	{named("GIF image"), gif.Is, gif.Scan, gif.Clean, gif.Fingerprint, "Every frame and its timing is byte-for-byte identical to the original."},
	{named("WebP image"), webp.Is, webp.Scan, webp.Clean, webp.Fingerprint, pictureProof},
	{named("PDF document"), pdf.Is, pdf.Scan, pdf.Clean, pdf.Fingerprint,
		"Every page, font and picture that the document uses is unchanged."},
	{office.Kind, office.Is, office.Scan, office.Clean, office.Fingerprint,
		"The text, formatting and pictures are unchanged apart from the names and IDs that were removed."},
}

// ErrUnsupported is returned for a file whose format has no handler.
var ErrUnsupported = errors.New("this file type is not supported. LazaScrub reads JPEG, PNG, GIF, WebP, PDF, Word, Excel and PowerPoint files")

func detect(data []byte) (handler, error) {
	for _, h := range handlers {
		if h.is(data) {
			return h, nil
		}
	}
	return handler{}, ErrUnsupported
}

// Report is what an inspection found in one file.
type Report struct {
	Path     string
	Format   string
	Size     int64
	SHA256   string
	Findings []meta.Finding
}

// Count returns how many findings have the given risk.
func (r *Report) Count(risk meta.Risk) int {
	n := 0
	for _, f := range r.Findings {
		if f.Risk == risk {
			n++
		}
	}
	return n
}

// Summary describes the report in one line.
func (r *Report) Summary() string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf("%s, %s. No metadata found.", r.Format, meta.Size(r.Size))
	}
	return fmt.Sprintf("%s, %s. %s found: %d high risk, %d medium, %d low.", r.Format, meta.Size(r.Size),
		plural(len(r.Findings), "field"), r.Count(meta.High), r.Count(meta.Medium), r.Count(meta.Low))
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func load(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("this is a folder. Choose a file, or use the folder section")
	}
	if info.Size() > MaxSize {
		return nil, errors.New("the file is larger than 1 GiB, which is the most LazaScrub reads")
	}
	return os.ReadFile(path)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// order puts the most revealing findings first and keeps each scanner's own
// order within a risk level.
func order(list []meta.Finding) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].Risk > list[j].Risk })
}

func inspect(path string, data []byte) (*Report, handler, error) {
	h, err := detect(data)
	if err != nil {
		return nil, h, err
	}
	found, err := h.scan(data)
	if err != nil {
		return nil, h, err
	}
	var list meta.List = found
	downloads(&list, path)
	order(list)
	return &Report{Path: path, Format: h.name(data), Size: int64(len(data)), SHA256: digest(data), Findings: list}, h, nil
}

// Inspect reads a file and lists its metadata. The file is not changed.
func Inspect(path string) (*Report, error) {
	data, err := load(path)
	if err != nil {
		return nil, err
	}
	report, _, err := inspect(path, data)
	return report, err
}

var zones = map[string]string{
	"0": "This computer", "1": "Local network", "2": "Trusted sites", "3": "The internet", "4": "Restricted sites",
}

// downloads reports the hidden streams that Windows keeps beside a file.
func downloads(list *meta.List, path string) {
	const note = "Windows keeps this in a hidden stream beside the file. It follows the file to other NTFS drives, " +
		"but not through email or uploads. A cleaned copy is a new file without it."
	for _, s := range ntfs.Streams(path) {
		if s.Name != "Zone.Identifier" {
			list.Add("Windows streams", "Hidden stream "+s.Name, meta.Size(s.Size), meta.Medium, note)
			continue
		}
		for _, line := range strings.Split(string(ntfs.Read(path, s.Name, 16<<10)), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok || value == "" {
				continue
			}
			switch key {
			case "ZoneId":
				if zone, known := zones[value]; known {
					value = zone
				}
				list.Add("Download record", "Came from", value, meta.Low, note)
			case "HostUrl":
				list.Add("Download record", "Downloaded from", value, meta.Medium, note)
			case "ReferrerUrl":
				list.Add("Download record", "Linked from page", value, meta.Medium, note)
			default:
				list.Add("Download record", key, value, meta.Low, note)
			}
		}
	}
}

// Result is the outcome of cleaning one file.
type Result struct {
	Source  *Report
	Output  *Report
	Removed []meta.Finding // in the original and gone from the copy
	Kept    []meta.Finding // left in place on purpose
	Left    []meta.Finding // still present although cleaning should have removed it
	Same    bool           // the content of the copy matches the original
	Proof   string         // what Same means for this format
}

// Summary describes the result in one or two sentences.
func (r *Result) Summary() string {
	total := len(r.Removed) + len(r.Kept) + len(r.Left)
	if total == 0 {
		return "The file had no metadata to remove."
	}
	s := fmt.Sprintf("Removed %d of %s.", len(r.Removed), plural(total, "field"))
	if len(r.Kept) > 0 {
		s += fmt.Sprintf(" %d kept on purpose.", len(r.Kept))
	}
	if len(r.Left) > 0 {
		s += fmt.Sprintf(" %d could not be removed.", len(r.Left))
	}
	return s
}

// Clean writes a cleaned copy of src to dst, then scans the copy to check the
// result. The original file is never modified.
func Clean(src, dst string) (*Result, error) {
	if same(src, dst) {
		return nil, errors.New("choose a different name for the cleaned copy. LazaScrub never overwrites the original")
	}
	data, err := load(src)
	if err != nil {
		return nil, err
	}
	source, h, err := inspect(src, data)
	if err != nil {
		return nil, err
	}
	cleaned, err := h.clean(data)
	if err != nil {
		return nil, err
	}
	if err := write(dst, cleaned); err != nil {
		return nil, err
	}
	output, _, err := inspect(dst, cleaned)
	if err != nil {
		os.Remove(dst)
		return nil, fmt.Errorf("the cleaned copy could not be read back, so it was deleted: %w", err)
	}
	r := &Result{Source: source, Output: output, Proof: h.proof}
	before, _ := h.fingerprint(data)
	after, _ := h.fingerprint(cleaned)
	r.Same = before != nil && bytes.Equal(before, after)

	remaining := map[string]int{}
	for _, f := range output.Findings {
		remaining[f.Key()]++
	}
	for _, f := range source.Findings {
		switch {
		case remaining[f.Key()] == 0:
			r.Removed = append(r.Removed, f)
		case f.Kept:
			remaining[f.Key()]--
			r.Kept = append(r.Kept, f)
		default:
			remaining[f.Key()]--
			r.Left = append(r.Left, f)
		}
	}
	return r, nil
}

func same(a, b string) bool {
	x, err1 := os.Stat(a)
	y, err2 := os.Stat(b)
	if err1 == nil && err2 == nil {
		return os.SameFile(x, y)
	}
	p, _ := filepath.Abs(a)
	q, _ := filepath.Abs(b)
	return strings.EqualFold(p, q)
}

// write saves data through a temporary file, so a failure never leaves a
// half-written copy behind.
func write(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".lazascrub-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}
