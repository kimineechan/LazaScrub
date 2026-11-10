package pdf

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"lazascrub/internal/jpeg"
	"lazascrub/internal/meta"
	"lazascrub/internal/xmp"
)

// The characters that PDFDocEncoding maps differently from Latin-1, from 0x80 to 0xA0.
var docEncoding = []rune("•†‡…—–ƒ⁄‹›−‰„“”‘’‚™ﬁﬂŁŒŠŸŽıłœšž�€")

// text decodes a PDF text string.
func text(s String) string {
	switch {
	case len(s) >= 2 && s[0] == 0xFE && s[1] == 0xFF:
		return meta.Line(meta.UTF16(s[2:], true))
	case len(s) >= 2 && s[0] == 0xFF && s[1] == 0xFE:
		return meta.Line(meta.UTF16(s[2:], false))
	case len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF:
		return meta.Line(string(s[3:]))
	}
	r := make([]rune, len(s))
	for i, c := range s {
		if c >= 0x80 && c <= 0xA0 {
			r[i] = docEncoding[c-0x80]
		} else {
			r[i] = rune(c)
		}
	}
	return meta.Line(string(r))
}

// date turns "D:20260314092653+01'00'" into "2026-03-14 09:26:53 +01:00".
func date(s string) string {
	if !strings.HasPrefix(s, "D:") {
		return s
	}
	t := s[2:]
	n := 0
	for n < len(t) && n < 14 && t[n] >= '0' && t[n] <= '9' {
		n++
	}
	if n < 4 {
		return s
	}
	out := t[:4]
	for i, sep := range []string{"-", "-", " ", ":", ":"} {
		if at := 4 + 2*i; n >= at+2 {
			out += sep + t[at:at+2]
		}
	}
	zone := strings.ReplaceAll(t[n:], "'", "")
	switch {
	case strings.HasPrefix(zone, "Z"):
		out += " UTC"
	case len(zone) >= 3 && (zone[0] == '+' || zone[0] == '-'):
		out += " " + zone[:3] + ":"
		if len(zone) >= 5 {
			out += zone[3:5]
		} else {
			out += "00"
		}
	}
	return out
}

type label struct {
	name string
	risk meta.Risk
	note string
}

const (
	noteTyped = "Text stored in the document properties."
	noteApp   = "Names the app that made the document, often with its version."
	noteWhen  = "Shows when the document was made or changed."
)

var infoLabels = map[Name]label{
	"Title":        {"Title", meta.Medium, noteTyped},
	"Author":       {"Author", meta.High, "Names the person who wrote the document."},
	"Subject":      {"Subject", meta.Medium, noteTyped},
	"Keywords":     {"Keywords", meta.Medium, noteTyped},
	"Creator":      {"Created with", meta.Medium, noteApp},
	"Producer":     {"PDF producer", meta.Medium, noteApp},
	"CreationDate": {"Created", meta.Medium, noteWhen},
	"ModDate":      {"Modified", meta.Medium, noteWhen},
	"Trapped":      {"Trapped", meta.Low, ""},
}

// Annotation types that carry a reviewer's name.
var markupTypes = map[Name]bool{
	"Text": true, "FreeText": true, "Line": true, "Square": true, "Circle": true, "Polygon": true,
	"PolyLine": true, "Highlight": true, "Underline": true, "Squiggly": true, "StrikeOut": true,
	"Stamp": true, "Caret": true, "Ink": true, "FileAttachment": true, "Sound": true, "Redact": true,
	"Popup": true,
}

func markup(d *Dict) bool {
	subtype, _ := d.Get("Subtype").(Name)
	return markupTypes[subtype] && (d.has("Rect") || d.Get("Type") == Name("Annot"))
}

// removable reports whether cleaning drops a dictionary key.
func removable(d *Dict, key Name, isMarkup bool) bool {
	switch key {
	case "Metadata", "PieceInfo", "LastModified", "Thumb":
		return true
	case "T", "M", "CreationDate":
		return isMarkup
	}
	return false
}

// soleFilter returns the filter of a stream that uses exactly one.
func soleFilter(d *Dict) Name {
	filters := list(d.Get("Filter"))
	if len(filters) != 1 {
		return ""
	}
	name, _ := filters[0].(Name)
	return name
}

type scanner struct {
	d       *document
	out     meta.List
	current map[string]bool // values that the current version of the document shows
	secret  bool            // the document is encrypted
}

// field renders one entry of a document information dictionary.
func (s *scanner) field(e entry) (label, string, bool) {
	l, known := infoLabels[e.key]
	var value string
	switch v := s.d.resolve(e.value).(type) {
	case String:
		if s.secret {
			value = "(encrypted)"
		} else {
			value = date(text(v))
		}
	case Name:
		value = string(v)
	case int64, float64, bool:
		value = fmt.Sprint(v)
	default:
		return l, "", false
	}
	if value == "" {
		return l, "", false
	}
	if !known {
		l = label{string(e.key), max(meta.Rate(string(e.key), value), meta.Medium), "A custom property added by the app that made the document."}
	}
	return l, value, true
}

// Scan lists the metadata in a PDF file.
func Scan(data []byte) ([]meta.Finding, error) {
	d, err := load(data)
	if err != nil {
		return nil, err
	}
	s := &scanner{d: d, current: map[string]bool{}, secret: d.trailer("Encrypt") != nil}
	s.information()
	s.objects()
	s.structure()
	if i := bytes.LastIndex(data, []byte("%%EOF")); i >= 0 {
		s.out.Hidden(bytes.TrimSpace(data[i+len("%%EOF"):]))
	}
	return s.out, nil
}

func (s *scanner) information() {
	if info := s.d.dict(s.d.trailer("Info")); info != nil {
		for _, e := range info.entries {
			if l, value, ok := s.field(e); ok {
				s.out.Add("Document information", l.name, value, l.risk, l.note)
				s.current[l.name+"\x00"+value] = true
			}
		}
	}
	if packet, ok := s.packet(s.d.catalog().Get("Metadata")); ok {
		for _, f := range xmp.Parse(packet, "XMP") {
			s.out = append(s.out, f)
			s.current[f.Field+"\x00"+f.Value] = true
		}
	}
	if s.secret {
		s.out.Keep("Security", "Encryption", "The document is password protected", meta.Low,
			"Text properties are stored encrypted, so they cannot be read or removed. Remove the password first.")
	}
	if ids, ok := s.d.resolve(s.d.trailer("ID")).(Array); ok && len(ids) == 2 {
		first, _ := ids[0].(String)
		second, _ := ids[1].(String)
		value := hex.EncodeToString(first)
		if !bytes.Equal(first, second) {
			value += " / " + hex.EncodeToString(second)
		}
		s.out.Add("File structure", "Document ID", value, meta.Medium,
			"Identifies this document. The first half stays the same across edits, so it links versions of one file.")
	}
}

// packet returns the decoded XMP stream that a reference points at.
func (s *scanner) packet(ref Object) ([]byte, bool) {
	r, ok := ref.(Ref)
	if !ok {
		return nil, false
	}
	o, err := s.d.get(r.Num)
	if err != nil || !o.isStream {
		return nil, false
	}
	return s.decodePacket(o)
}

func (s *scanner) decodePacket(o *object) ([]byte, bool) {
	packet, err := decode(o.value.(*Dict), o.stream)
	if err != nil {
		s.out.Add("XMP", "XMP packet", meta.Size(int64(len(o.stream)))+" that could not be decoded", meta.Medium, "")
		return nil, false
	}
	return packet, true
}

// objects looks through every object the document uses.
func (s *scanner) objects() {
	main := -1
	if r, ok := s.d.catalog().Get("Metadata").(Ref); ok {
		main = r.Num
	}
	authors := map[string]int{}
	var files []string
	scripts, pieces, thumbs, signatures := 0, 0, 0, 0
	for _, num := range s.d.numbers() {
		o, err := s.d.get(num)
		if err != nil {
			continue
		}
		dict, _ := o.value.(*Dict)
		if dict == nil {
			continue
		}
		kind := dict.Get("Type")
		if o.isStream && kind == Name("Metadata") && num != main {
			if packet, ok := s.decodePacket(o); ok {
				s.out.Nest(fmt.Sprintf("Object %d", num), xmp.Parse(packet, "XMP"))
			}
		}
		if o.isStream && soleFilter(dict) == "DCTDecode" {
			if inner, err := jpeg.Scan(o.stream); err == nil {
				var leaks []meta.Finding
				for _, f := range inner {
					if !f.Kept {
						leaks = append(leaks, f)
					}
				}
				s.out.Nest(fmt.Sprintf("Picture in object %d", num), leaks)
			}
		}
		if kind == Name("Filespec") || dict.has("EF") {
			name, _ := s.d.resolve(dict.Get("UF")).(String)
			if name == nil {
				name, _ = s.d.resolve(dict.Get("F")).(String)
			}
			files = append(files, text(name))
		}
		if dict.Get("S") == Name("JavaScript") || dict.has("JS") {
			scripts++
		}
		if markup(dict) {
			if author, ok := s.d.resolve(dict.Get("T")).(String); ok && !s.secret && len(author) > 0 {
				authors[text(author)]++
			}
		}
		if dict.has("PieceInfo") {
			pieces++
		}
		if dict.has("Thumb") {
			thumbs++
		}
		if kind == Name("Sig") || (dict.Get("FT") == Name("Sig") && dict.has("V")) {
			signatures++
		}
	}
	names := make([]string, 0, len(authors))
	for name := range authors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s.out.Add("Comments", "Comment author", fmt.Sprintf("%s (%s)", name, plural(authors[name], "comment")), meta.High,
			"The name shown on review comments and highlights.")
	}
	if thumbs > 0 {
		s.out.Add("Thumbnails", "Page thumbnails", plural(thumbs, "page"), meta.High,
			"Small pictures of the pages. They can show a page as it looked before it was edited.")
	}
	if pieces > 0 {
		s.out.Add("Private app data", "Editing data", plural(pieces, "block"), meta.Medium,
			"Private data from the app that made the file, such as its editing state.")
	}
	for i, name := range files {
		if i == 50 {
			s.out.Keep("Attachments", "Embedded file", fmt.Sprintf("and %d more", len(files)-50), meta.Medium, "")
			break
		}
		s.out.Keep("Attachments", "Embedded file", name, meta.Medium,
			"A file attached inside the PDF. It is part of the document, so it is not removed, and it can carry metadata of its own.")
	}
	if scripts > 0 {
		s.out.Keep("Active content", "JavaScript", plural(scripts, "script"), meta.Medium,
			"Scripts that run when the document is opened or used. They are part of the document, so they are not removed.")
	}
	if signatures > 0 {
		s.out.Keep("Security", "Digital signature", plural(signatures, "signature"), meta.Low,
			"Cleaning rewrites the file, which invalidates a digital signature.")
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// structure reports what earlier saves left behind in the file.
func (s *scanner) structure() {
	if s.d.revisions > 1 {
		s.out.Add("Earlier versions", "Saved revisions", plural(s.d.revisions-1, "earlier version")+" of the document still in the file", meta.High,
			"PDF editors append changes instead of rewriting the file. Text, pictures and properties that were replaced or deleted can be recovered from the earlier versions.")
	}
	dead, size := 0, 0
	for _, sp := range s.d.spots {
		pl, listed := s.d.table[sp.num]
		live := listed && pl.holder == 0 && (pl.offset == sp.offset || pl.offset+s.d.base == sp.offset)
		dict, _ := sp.obj.value.(*Dict)
		if sp.obj.isStream && dict.Get("Type") == Name("ObjStm") {
			if h, err := s.d.openHolder(sp.obj); err == nil {
				for i, num := range h.nums {
					if at, ok := s.d.table[num]; live && ok && at.holder == sp.num {
						continue
					}
					dead++
					if member, err := h.member(i); err == nil {
						size += member.end - member.start
						s.old(member)
					}
				}
			}
		}
		if live || dict.Get("Type") == Name("XRef") || dict.Get("Type") == Name("ObjStm") {
			continue
		}
		dead++
		size += sp.obj.next - sp.offset
		s.old(sp.obj)
	}
	if dead > 0 {
		s.out.Add("Earlier versions", "Replaced objects", fmt.Sprintf("%s, %s, no longer used by the document", plural(dead, "object"), meta.Size(int64(size))), meta.High,
			"Left behind by edits. They can hold content that was replaced or deleted.")
	}
}

// old reports properties found in an object that a later save replaced.
func (s *scanner) old(o *object) {
	const note = "Replaced by a later edit, but still stored in the file."
	dict, _ := o.value.(*Dict)
	if dict == nil || s.secret {
		return
	}
	if o.isStream {
		if dict.Get("Type") != Name("Metadata") {
			return
		}
		if packet, err := decode(dict, o.stream); err == nil {
			for _, f := range xmp.Parse(packet, "Earlier version > XMP") {
				if key := f.Field + "\x00" + f.Value; !s.current[key] {
					s.current[key] = true
					f.Note = note
					s.out = append(s.out, f)
				}
			}
		}
		return
	}
	if !dict.has("Producer") && !dict.has("CreationDate") && !dict.has("ModDate") && !dict.has("Author") {
		return
	}
	for _, e := range dict.entries {
		if l, value, ok := s.field(e); ok && !s.current[l.name+"\x00"+value] {
			s.current[l.name+"\x00"+value] = true
			s.out.Add("Earlier version > Document information", l.name, value, l.risk, note)
		}
	}
}
