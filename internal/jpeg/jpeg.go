// Package jpeg finds and removes metadata in JPEG files.
package jpeg

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"lazascrub/internal/exif"
	"lazascrub/internal/icc"
	"lazascrub/internal/iptc"
	"lazascrub/internal/meta"
	"lazascrub/internal/xmp"
)

const (
	sigExif     = "Exif\x00\x00"
	sigXMP      = "http://ns.adobe.com/xap/1.0/\x00"
	sigExtended = "http://ns.adobe.com/xmp/extension/\x00"
	sigICC      = "ICC_PROFILE\x00"
	sigMPF      = "MPF\x00"
	sigIRB      = "Photoshop 3.0\x00"
	sigJFIF     = "JFIF\x00"
	sigJFXX     = "JFXX\x00"
	sigAdobe    = "Adobe"
)

// part is one segment of the file. For a start-of-scan segment, data also
// holds the compressed picture data that follows it.
type part struct {
	marker byte
	data   []byte // the whole segment as it appears in the file
	body   []byte // the payload after the length field
}

type file struct {
	parts    []part
	trailing []byte // bytes after the end-of-image marker
}

// Is reports whether data starts like a JPEG file.
func Is(data []byte) bool {
	return len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}

func parse(data []byte) (*file, error) {
	if !Is(data) {
		return nil, errors.New("not a JPEG file")
	}
	f := &file{}
	pos := 2
	for pos < len(data) {
		start := pos
		if data[pos] != 0xFF {
			break
		}
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			break
		}
		m := data[pos]
		pos++
		if m == 0xD9 {
			f.parts = append(f.parts, part{marker: m, data: data[start:pos]})
			f.trailing = data[pos:]
			return f, nil
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			f.parts = append(f.parts, part{marker: m, data: data[start:pos]})
			continue
		}
		if pos+2 > len(data) {
			pos = start
			break
		}
		size := int(data[pos])<<8 | int(data[pos+1])
		if size < 2 || pos+size > len(data) {
			pos = start
			break
		}
		end := pos + size
		p := part{marker: m, body: data[pos+2 : end]}
		if m == 0xDA {
			end = scanEnd(data, end)
		}
		p.data = data[start:end]
		f.parts = append(f.parts, p)
		pos = end
	}
	// The file is damaged or cut short. Keep the rest untouched.
	if pos < len(data) {
		f.parts = append(f.parts, part{data: data[pos:]})
	}
	return f, nil
}

// scanEnd returns the offset of the first marker after compressed picture data.
func scanEnd(data []byte, pos int) int {
	for {
		i := bytes.IndexByte(data[pos:], 0xFF)
		if i < 0 || pos+i+1 >= len(data) {
			return len(data)
		}
		pos += i
		next := data[pos+1]
		switch {
		case next == 0xFF:
			pos++
		case next == 0x00 || (next >= 0xD0 && next <= 0xD7):
			pos += 2
		default:
			return pos
		}
	}
}

func (p part) is(marker byte, signature string) bool {
	return p.marker == marker && strings.HasPrefix(string(p.body[:min(len(p.body), len(signature))]), signature)
}

// metadata reports whether a segment carries metadata rather than picture data.
func (p part) metadata() bool {
	return (p.marker >= 0xE0 && p.marker <= 0xEF) || p.marker == 0xFE
}

// Scan lists the metadata in a JPEG file.
func Scan(data []byte) ([]meta.Finding, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	var out meta.List
	profile := map[int][]byte{}
	var extended []byte
	multi := false
	for _, p := range f.parts {
		switch {
		case p.is(0xE0, sigJFIF) && len(p.body) >= 14:
			unit := [...]string{"(aspect ratio)", "dpi", "dots per cm"}[min(int(p.body[7]), 2)]
			x, y := int(p.body[8])<<8|int(p.body[9]), int(p.body[10])<<8|int(p.body[11])
			out.Keep("JFIF header", "Resolution", fmt.Sprintf("%d x %d %s", x, y, unit), meta.Low, meta.NoteNeeded)
			if p.body[12] != 0 && p.body[13] != 0 {
				out.Add("JFIF header", "Thumbnail", fmt.Sprintf("%d x %d preview image", p.body[12], p.body[13]), meta.High, meta.NotePreview)
			}
		case p.is(0xE0, sigJFXX):
			out.Add("JFIF header", "Thumbnail", meta.Size(int64(len(p.body)))+" preview image", meta.High, meta.NotePreview)
		case p.is(0xE1, sigExif):
			info, err := exif.Parse(p.body[len(sigExif):])
			if err != nil {
				out.Add("EXIF", "EXIF block", meta.Size(int64(len(p.body)))+" that could not be read", meta.Medium,
					"Camera data in a damaged or unusual layout. It is removed as a whole.")
			}
			out = append(out, info.Findings...)
		case p.is(0xE1, sigXMP):
			out = append(out, xmp.Parse(p.body[len(sigXMP):], "XMP")...)
		case p.is(0xE1, sigExtended) && len(p.body) >= len(sigExtended)+40:
			chunk := p.body[len(sigExtended)+32:]
			total, offset := int(binary.BigEndian.Uint32(chunk)), int(binary.BigEndian.Uint32(chunk[4:]))
			if total > 0 && total <= 64<<20 && offset >= 0 && offset+len(chunk)-8 <= total {
				if extended == nil {
					extended = make([]byte, total)
				}
				if len(extended) == total {
					copy(extended[offset:], chunk[8:])
				}
			}
		case p.is(0xE2, sigICC) && len(p.body) > len(sigICC)+2:
			profile[int(p.body[len(sigICC)])] = p.body[len(sigICC)+2:]
		case p.is(0xE2, sigMPF):
			multi = true
			value := "Extra pictures are listed"
			if n := mpfCount(p.body[len(sigMPF):]); n > 1 {
				value = fmt.Sprintf("%d pictures in one file", n)
			}
			out.Add("Multi-picture", "Picture index", value, meta.Medium,
				"The file holds more than one picture, such as a depth map, a gain map or a larger preview.")
		case p.is(0xED, sigIRB):
			out = append(out, iptc.Parse(p.body[len(sigIRB):])...)
		case p.is(0xEE, sigAdobe):
			out.Keep("Adobe marker", "Colour transform", fmt.Sprintf("%d bytes", len(p.body)), meta.Low, meta.NoteNeeded)
		case p.marker == 0xFE:
			out.Add("Comment", "Comment", meta.Text(p.body), meta.Medium, "Free text stored by the app that saved the file.")
		case p.marker >= 0xE0 && p.marker <= 0xEF:
			name, note := describe(p)
			out.Add("App segment", name, meta.Size(int64(len(p.body))), meta.Medium, note)
		}
	}
	if len(profile) > 0 {
		keys := make([]int, 0, len(profile))
		for k := range profile {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		var whole []byte
		for _, k := range keys {
			whole = append(whole, profile[k]...)
		}
		out = append(out, icc.Parse(whole)...)
	}
	if extended != nil {
		out = append(out, xmp.Parse(extended, "XMP (extended)")...)
	}
	if multi && Is(f.trailing) {
		out.Add("After the content", "Hidden data", meta.Size(int64(len(f.trailing)))+" of extra pictures", meta.High, meta.NoteHidden)
	} else {
		out.Hidden(f.trailing)
	}
	return out, nil
}

// describe names an application segment that has no dedicated reader.
func describe(p part) (string, string) {
	signature := p.body
	for i, c := range signature {
		if c < 0x20 || c > 0x7E || i == 24 {
			signature = signature[:i]
			break
		}
	}
	name := fmt.Sprintf("APP%d", p.marker-0xE0)
	if len(signature) > 1 {
		name += " " + string(signature)
	}
	switch {
	case p.marker == 0xEB && strings.HasPrefix(string(signature), "JP"):
		return name, "Boxed data such as content credentials, which can record the tools or AI model that made the picture."
	case string(signature) == "Ducky":
		return name, "Export settings written by Adobe Save for Web."
	}
	return name, meta.NoteExtra
}

// mpfCount reads the number of pictures from a multi-picture index.
func mpfCount(b []byte) int {
	if len(b) < 8 {
		return 0
	}
	var order binary.ByteOrder = binary.LittleEndian
	if string(b[:2]) == "MM" {
		order = binary.BigEndian
	}
	at := uint64(order.Uint32(b[4:]))
	if at+2 > uint64(len(b)) {
		return 0
	}
	n := uint64(order.Uint16(b[at:]))
	for i := uint64(0); i < n && at+2+12*i+12 <= uint64(len(b)); i++ {
		e := b[at+2+12*i:]
		if order.Uint16(e) == 0xB001 {
			return int(order.Uint32(e[8:]))
		}
	}
	return 0
}

func segment(out *bytes.Buffer, marker byte, body []byte) {
	out.Write([]byte{0xFF, marker, byte((len(body) + 2) >> 8), byte(len(body) + 2)})
	out.Write(body)
}

// Clean returns a copy of the file without its metadata. Picture data, the
// colour profile and the display orientation are carried over unchanged.
func Clean(data []byte) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	orientation := 0
	for _, p := range f.parts {
		if p.is(0xE1, sigExif) {
			if info, err := exif.Parse(p.body[len(sigExif):]); err == nil && info.Orientation >= 2 && info.Orientation <= 8 {
				orientation = info.Orientation
			}
		}
	}
	var out bytes.Buffer
	out.Grow(len(data))
	out.Write([]byte{0xFF, 0xD8})
	keepOrientation := func() {
		if orientation != 0 {
			segment(&out, 0xE1, append([]byte(sigExif), exif.Orientation(orientation)...))
			orientation = 0
		}
	}
	for _, p := range f.parts {
		switch {
		case p.is(0xE0, sigJFIF) && len(p.body) >= 14:
			body := append([]byte(nil), p.body[:14]...)
			body[12], body[13] = 0, 0
			segment(&out, 0xE0, body)
			keepOrientation()
		case p.is(0xE2, sigICC), p.is(0xEE, sigAdobe):
			keepOrientation()
			out.Write(p.data)
		case p.metadata():
		default:
			keepOrientation()
			out.Write(p.data)
		}
	}
	return out.Bytes(), nil
}

// Fingerprint hashes the picture data alone, so two files with the same
// fingerprint decode to the same picture whatever metadata they carry.
func Fingerprint(data []byte) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	for _, p := range f.parts {
		if !p.metadata() || p.is(0xEE, sigAdobe) {
			h.Write(p.data)
		}
	}
	return h.Sum(nil), nil
}
