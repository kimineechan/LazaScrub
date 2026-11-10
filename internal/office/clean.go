package office

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"regexp"
	"strings"

	"lazascrub/internal/gif"
	"lazascrub/internal/jpeg"
	"lazascrub/internal/pdf"
	"lazascrub/internal/png"
	"lazascrub/internal/webp"
)

const quoted = `(?:"[^"]*"|'[^']*')`

var (
	// Word: reviewer names and dates on comments and tracked changes, and
	// the session IDs that link documents to each other.
	wordAuthor   = regexp.MustCompile(`(\sw:author\s*=\s*)` + quoted)
	wordInitials = regexp.MustCompile(`(\sw:initials\s*=\s*)` + quoted)
	wordDates    = regexp.MustCompile(`\s(?:w:date|w16du:dateUtc)\s*=\s*` + quoted)
	wordSession  = regexp.MustCompile(`\sw:rsid[A-Za-z]*\s*=\s*` + quoted)
	wordSessions = regexp.MustCompile(`(?s)<w:rsids>.*?</w:rsids>`)
	wordTemplate = regexp.MustCompile(`<w:attachedTemplate\b[^<>]*/>`)
	wordPrinter  = regexp.MustCompile(`<w:printerSettings\b[^<>]*/>`)

	// Excel: the folder the workbook was saved in, and the link from a
	// sheet to its printer settings.
	savedIn     = regexp.MustCompile(`(?s)<mc:AlternateContent\b[^<>]*>\s*<mc:Choice\b[^<>]*>\s*<x15ac:absPath\b[^<>]*/>\s*</mc:Choice>\s*</mc:AlternateContent>`)
	savedInBare = regexp.MustCompile(`<x15ac:absPath\b[^<>]*/>`)
	pageSetup   = regexp.MustCompile(`<pageSetup\b[^<>]*>`)
	relationID  = regexp.MustCompile(`\sr:id\s*=\s*` + quoted)

	// Package: the relationships and content types of the parts that are removed.
	removedLink = regexp.MustCompile(`<Relationship\b[^<>]*\bType\s*=\s*"[^"]*/(?:printerSettings|attachedTemplate|metadata/thumbnail)"[^<>]*(?:/>|>\s*</Relationship>)`)
	removedType = regexp.MustCompile(`<Override\b[^<>]*\bPartName\s*=\s*"/(?:docProps/thumbnail\.[^"]*|[^"]*/printerSettings/[^"]*)"[^<>]*/>`)

	attribute = map[string]*regexp.Regexp{}
)

func init() {
	for _, name := range []string{"name", "initials", "displayName", "userId", "providerId"} {
		attribute[name] = regexp.MustCompile(`(\s` + name + `\s*=\s*)` + quoted)
	}
}

// set rewrites attributes on every tag of one element.
func set(data []byte, element string, values map[string]string) []byte {
	prefix := []byte("<" + element)
	return tagPattern.ReplaceAllFunc(data, func(tag []byte) []byte {
		if !bytes.HasPrefix(tag, prefix) || (len(tag) > len(prefix) && isNameByte(tag[len(prefix)])) {
			return tag
		}
		for name, value := range values {
			tag = attribute[name].ReplaceAll(tag, []byte(`${1}"`+value+`"`))
		}
		return tag
	})
}

var nobody = map[string]string{"name": anonymous, "displayName": anonymous, "initials": "A", "userId": anonymous, "providerId": "None"}

// transform returns the cleaned content of one part. It reports drop when
// the part is left out of the cleaned package altogether.
func transform(name string, data []byte, depth int) (out []byte, drop bool) {
	xml := strings.HasSuffix(name, ".xml")
	switch {
	case name == "docProps/core.xml":
		return []byte(emptyCore), false
	case name == "docProps/app.xml":
		return []byte(emptyApp), false
	case name == "docProps/custom.xml":
		return []byte(emptyCustom), false
	case strings.HasPrefix(name, "docProps/thumbnail"), strings.Contains(name, "/printerSettings/"):
		return nil, true
	case name == "word/people.xml":
		return []byte(emptyPeople), false
	case strings.HasSuffix(name, ".rels"):
		return removedLink.ReplaceAll(data, nil), false
	case name == "[Content_Types].xml":
		return removedType.ReplaceAll(data, nil), false
	case strings.HasPrefix(name, "word/") && xml:
		data = wordAuthor.ReplaceAll(data, []byte(`${1}"`+anonymous+`"`))
		data = wordInitials.ReplaceAll(data, []byte(`${1}"A"`))
		data = wordDates.ReplaceAll(data, nil)
		data = wordSession.ReplaceAll(data, nil)
		data = wordSessions.ReplaceAll(data, nil)
		data = wordTemplate.ReplaceAll(data, nil)
		return wordPrinter.ReplaceAll(data, nil), false
	case name == "xl/workbook.xml":
		return savedInBare.ReplaceAll(savedIn.ReplaceAll(data, nil), nil), false
	case strings.HasPrefix(name, "xl/") && strings.Contains(name, "sheets/") && xml:
		return pageSetup.ReplaceAllFunc(data, func(tag []byte) []byte { return relationID.ReplaceAll(tag, nil) }), false
	case strings.HasPrefix(name, "xl/comments") && xml:
		return authorList.ReplaceAllFunc(data, func(block []byte) []byte {
			return authorItem.ReplaceAll(block, []byte("<author>"+anonymous+"</author>"))
		}), false
	case strings.HasPrefix(name, "xl/persons/") && xml:
		return set(data, "person", nobody), false
	case name == "ppt/authors.xml":
		return set(data, "p188:author", nobody), false
	case name == "ppt/commentAuthors.xml":
		return set(data, "p:cmAuthor", nobody), false
	case strings.Contains(name, "/media/") || strings.Contains(name, "/embeddings/"):
		return inner(data, depth), false
	}
	return data, false
}

// inner cleans a picture or document stored inside the package. Anything
// that cannot be cleaned is passed through untouched.
func inner(data []byte, depth int) []byte {
	var out []byte
	var err error
	switch {
	case jpeg.Is(data):
		out, err = jpeg.Clean(data)
	case png.Is(data):
		out, err = png.Clean(data)
	case gif.Is(data):
		out, err = gif.Clean(data)
	case webp.Is(data):
		out, err = webp.Clean(data)
	case pdf.Is(data):
		out, err = pdf.Clean(data)
	case depth < maxDepth && Is(data):
		out, err = clean(data, depth+1)
	default:
		return data
	}
	if err != nil {
		return data
	}
	return out
}

// Clean returns a copy of the document without its metadata. Property parts
// are emptied, the thumbnail and printer settings are removed, reviewer
// names are replaced with "Author", and embedded pictures are cleaned. Text,
// formatting, comments and tracked changes themselves are left as they are.
func Clean(data []byte) ([]byte, error) {
	return clean(data, 0)
}

func clean(data []byte, depth int) ([]byte, error) {
	r, err := open(data)
	if err != nil {
		return nil, err
	}
	if kind(r) == "" {
		return nil, errors.New("not an Office document")
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range r.File {
		// Every part gets the fixed date that Office itself writes.
		header := &zip.FileHeader{Name: f.Name, Method: f.Method, ModifiedDate: 0x0021}
		original := read(f)
		out, drop := transform(f.Name, original, depth)
		if drop {
			continue
		}
		if bytes.Equal(out, original) {
			// Unchanged parts are copied as stored, without recompressing.
			header.Flags = f.Flags &^ 0x8
			header.CRC32, header.CompressedSize64, header.UncompressedSize64 = f.CRC32, f.CompressedSize64, f.UncompressedSize64
			raw, err := f.OpenRaw()
			if err != nil {
				return nil, err
			}
			dst, err := w.CreateRaw(header)
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(dst, raw); err != nil {
				return nil, err
			}
			continue
		}
		header.Method = zip.Deflate
		dst, err := w.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := dst.Write(out); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Fingerprint hashes the content of the document: every part that Clean
// keeps, in the form Clean leaves it.
func Fingerprint(data []byte) ([]byte, error) {
	r, err := open(data)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "docProps/") || f.Name == "word/people.xml" {
			continue
		}
		out, drop := transform(f.Name, read(f), 0)
		if drop {
			continue
		}
		sum := sha256.Sum256(out)
		io.WriteString(h, f.Name+"\x00")
		h.Write(sum[:])
	}
	return h.Sum(nil), nil
}
