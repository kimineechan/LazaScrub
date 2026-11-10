// Package meta defines the findings that every format scanner reports.
package meta

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Risk says how much a piece of metadata gives away about a person or place.
type Risk int

const (
	Low Risk = iota
	Medium
	High
)

func (r Risk) String() string {
	switch r {
	case High:
		return "High"
	case Medium:
		return "Medium"
	}
	return "Low"
}

// Finding is one piece of metadata found in a file.
type Finding struct {
	Section string // where it is stored, such as "EXIF" or "PDF information"
	Field   string
	Value   string
	Risk    Risk
	Note    string // why it matters, or why cleaning leaves it in place
	Kept    bool   // cleaning leaves this in place on purpose
}

// Key identifies a finding when a file is compared before and after cleaning.
func (f Finding) Key() string { return f.Section + "\x00" + f.Field + "\x00" + f.Value }

// List collects findings in the order a scanner meets them.
type List []Finding

func (l *List) Add(section, field, value string, risk Risk, note string) {
	*l = append(*l, Finding{Section: section, Field: field, Value: value, Risk: risk, Note: note})
}

// Keep records something the cleaner leaves in place, with the reason.
func (l *List) Keep(section, field, value string, risk Risk, note string) {
	*l = append(*l, Finding{Section: section, Field: field, Value: value, Risk: risk, Note: note, Kept: true})
}

// Nest adds findings from an embedded item, prefixing where they were found.
func (l *List) Nest(prefix string, inner []Finding) {
	for _, f := range inner {
		f.Section = prefix + " > " + f.Section
		*l = append(*l, f)
	}
}

// Explanations shared by several formats.
const (
	NotePreview = "A small copy of the picture. It can still show areas that were cropped or edited out later."
	NoteHidden  = "Data stored after the end of the real content. Viewers ignore it, so it travels with the file unnoticed."
	NoteNeeded  = "Kept because the picture needs it to display correctly."
	NoteExtra   = "Extra data added by the camera or an app. Viewers ignore it."
)

// Hidden records bytes found after the end of a file's real content.
func (l *List) Hidden(trailing []byte) {
	if len(trailing) == 0 {
		return
	}
	value := fmt.Sprintf("%s, looks like %s", Size(int64(len(trailing))), Sniff(trailing))
	l.Add("After the content", "Hidden data", value, High, NoteHidden)
}

var (
	emailPattern = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
	pathMarks    = []string{"file://", `:\users\`, `:\documents and settings\`, "/users/", "/home/", `\\`}
)

// Rate estimates the risk of a field from its name and its value. A value
// that holds a file path or an email address is always high risk, because
// both tend to contain a person's name.
func Rate(name, value string) Risk {
	risk := RiskOf(name)
	if risk == High {
		return High
	}
	lower := strings.ToLower(value)
	for _, mark := range pathMarks {
		if strings.Contains(lower, mark) {
			return High
		}
	}
	if emailPattern.MatchString(value) {
		return High
	}
	return risk
}

// Words that mark a field as identifying a person, a place or a device.
var highWords = []string{
	"gps", "latitude", "longitude", "location", "city", "country", "state", "province", "address",
	"author", "creator", "artist", "owner", "byline", "by-line", "credit", "contact", "email", "e-mail",
	"phone", "person", "people", "user", "manager", "company", "copyright", "rights", "serial",
	"lastmodifiedby", "last modified by", "hostcomputer", "host computer", "path", "url",
}

var mediumWords = []string{
	"date", "time", "created", "modified", "software", "tool", "producer", "application", "program",
	"agent", "make", "model", "device", "lens", "documentid", "instanceid", "uuid", "guid", "history",
	"title", "subject", "keyword", "description", "comment", "caption", "headline", "template",
	"revision", "version", "identifier", "label", "category", "source", "note", "prompt", "parameters",
}

// RiskOf estimates the risk of a field from its name. Scanners use it for
// free-form fields that no table describes.
func RiskOf(name string) Risk {
	n := strings.ToLower(name)
	for _, w := range highWords {
		if strings.Contains(n, w) {
			return High
		}
	}
	for _, w := range mediumWords {
		if strings.Contains(n, w) {
			return Medium
		}
	}
	return Low
}

// Text turns raw bytes into one clean line. It accepts UTF-8, UTF-16 with a
// byte order mark and Latin-1, and replaces control characters with spaces.
func Text(b []byte) string {
	var s string
	switch {
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		s = UTF16(b[2:], true)
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		s = UTF16(b[2:], false)
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		s = string(b[3:])
	case utf8.Valid(b):
		s = string(b)
	default:
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		s = string(r)
	}
	return Line(s)
}

// UTF16 decodes UTF-16 bytes in the given byte order.
func UTF16(b []byte, bigEndian bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		}
	}
	return string(utf16.Decode(u))
}

// Line collapses a string to a single trimmed line.
func Line(s string) string {
	var out strings.Builder
	space := false
	for _, r := range s {
		if r == 0 || r == utf8.RuneError {
			continue
		}
		if r < 0x20 || r == 0x7F || r == ' ' || r == ' ' {
			r = ' '
		}
		if r == ' ' {
			if space {
				continue
			}
			space = true
		} else {
			space = false
		}
		out.WriteRune(r)
	}
	return strings.TrimSpace(out.String())
}

// Clip shortens a value for display.
func Clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

// Size formats a byte count the way Explorer does.
func Size(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d bytes", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
}

// Sniff names well-known file types from their first bytes. It is used to
// describe data that should not be there, such as bytes after the end of an image.
func Sniff(b []byte) string {
	has := func(sig string) bool { return len(b) >= len(sig) && string(b[:len(sig)]) == sig }
	switch {
	case has("\xFF\xD8\xFF"):
		return "a JPEG image"
	case has("\x89PNG\r\n\x1a\n"):
		return "a PNG image"
	case has("PK\x03\x04"):
		return "a ZIP archive"
	case has("Rar!"):
		return "a RAR archive"
	case has("7z\xBC\xAF\x27\x1C"):
		return "a 7-Zip archive"
	case has("%PDF-"):
		return "a PDF document"
	case has("MZ"):
		return "a Windows program"
	case has("GIF8"):
		return "a GIF image"
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		return "an MP4 or QuickTime video"
	case has("\x1F\x8B"):
		return "gzip-compressed data"
	}
	zero := true
	for _, c := range b {
		if c != 0 {
			zero = false
			break
		}
	}
	if zero {
		return "padding (zero bytes)"
	}
	return "unrecognised data"
}

// Lookup finds the first finding with the given section and field.
func Lookup(list []Finding, section, field string) (Finding, bool) {
	for _, f := range list {
		if f.Section == section && f.Field == field {
			return f, true
		}
	}
	return Finding{}, false
}
