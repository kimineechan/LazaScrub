// Package gif finds and removes metadata in GIF files.
package gif

import (
	"bytes"
	"crypto/sha256"
	"errors"

	"lazascrub/internal/icc"
	"lazascrub/internal/meta"
	"lazascrub/internal/xmp"
)

const (
	blockImage     = 0x2C
	blockExtension = 0x21
	blockTrailer   = 0x3B

	labelText    = 0x01
	labelControl = 0xF9
	labelComment = 0xFE
	labelApp     = 0xFF
)

type block struct {
	kind  byte
	label byte   // extension label
	app   string // application identifier and code
	body  []byte // the sub-block area of an extension, lengths included
	raw   []byte
}

type file struct {
	head     []byte // header, screen descriptor and global colour table
	blocks   []block
	trailing []byte
}

// Is reports whether data starts like a GIF file.
func Is(data []byte) bool {
	return bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))
}

// skip walks a chain of length-prefixed sub-blocks and returns the offset after its terminator.
func skip(data []byte, pos int) (int, bool) {
	for pos < len(data) {
		n := int(data[pos])
		pos++
		if n == 0 {
			return pos, true
		}
		pos += n
	}
	return 0, false
}

func table(packed byte) int {
	if packed&0x80 == 0 {
		return 0
	}
	return 3 << ((packed & 7) + 1)
}

func parse(data []byte) (*file, error) {
	if !Is(data) || len(data) < 13 {
		return nil, errors.New("not a GIF file")
	}
	pos := 13 + table(data[10])
	if pos > len(data) {
		return nil, errors.New("the GIF header is cut short")
	}
	f := &file{head: data[:pos]}
	for pos < len(data) {
		start := pos
		switch data[pos] {
		case blockImage:
			if pos+10 > len(data) {
				break
			}
			at := pos + 10 + table(data[pos+9]) + 1
			end, ok := skip(data, at)
			if !ok {
				break
			}
			f.blocks = append(f.blocks, block{kind: blockImage, raw: data[start:end]})
			pos = end
			continue
		case blockExtension:
			if pos+2 > len(data) {
				break
			}
			end, ok := skip(data, pos+2)
			if !ok {
				break
			}
			b := block{kind: blockExtension, label: data[pos+1], body: data[pos+2 : end], raw: data[start:end]}
			if b.label == labelApp && len(b.body) >= 12 && b.body[0] == 11 {
				b.app = string(b.body[1:12])
				b.body = b.body[12:]
			}
			f.blocks = append(f.blocks, b)
			pos = end
			continue
		case blockTrailer:
			f.blocks = append(f.blocks, block{kind: blockTrailer, raw: data[start : start+1]})
			f.trailing = data[start+1:]
			return f, nil
		}
		break
	}
	// Damaged or cut short. Keep the rest untouched.
	if pos < len(data) {
		f.blocks = append(f.blocks, block{raw: data[pos:]})
	}
	return f, nil
}

// join strips the length bytes from a chain of sub-blocks.
func join(body []byte) []byte {
	var out []byte
	for pos := 0; pos < len(body); {
		n := int(body[pos])
		pos++
		if n == 0 || pos+n > len(body) {
			break
		}
		out = append(out, body[pos:pos+n]...)
		pos += n
	}
	return out
}

func (b block) keep() bool {
	if b.kind != blockExtension {
		return true
	}
	switch b.label {
	case labelControl, labelText:
		return true
	case labelApp:
		return b.app == "NETSCAPE2.0" || b.app == "ANIMEXTS1.0" || b.app == "ICCRGBG1012"
	}
	return false
}

// Scan lists the metadata in a GIF file.
func Scan(data []byte) ([]meta.Finding, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	var out meta.List
	for _, b := range f.blocks {
		if b.kind != blockExtension {
			continue
		}
		switch {
		case b.label == labelComment:
			out.Add("Comment", "Comment", meta.Text(join(b.body)), meta.Medium, "Free text stored by the app that saved the file.")
		case b.label == labelApp && b.app == "XMP DataXMP":
			// The packet is stored as plain bytes, followed by a 258-byte
			// trailer that makes it look like a sub-block chain.
			packet := b.body
			if len(packet) > 258 {
				packet = packet[:len(packet)-258]
			}
			out = append(out, xmp.Parse(packet, "XMP")...)
		case b.label == labelApp && b.app == "ICCRGBG1012":
			out = append(out, icc.Parse(join(b.body))...)
		case b.label == labelApp && !b.keep():
			out.Add("App extension", meta.Text([]byte(b.app)), meta.Size(int64(len(b.body))), meta.Medium, meta.NoteExtra)
		case !b.keep():
			out.Add("Extension", "Unknown extension", meta.Size(int64(len(b.body))), meta.Medium, meta.NoteExtra)
		}
	}
	out.Hidden(f.trailing)
	return out, nil
}

// Clean returns a copy of the file without comments, XMP and other
// application data. Frames, timing and looping are carried over unchanged.
func Clean(data []byte) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(len(data))
	out.Write(f.head)
	for _, b := range f.blocks {
		if b.keep() {
			out.Write(b.raw)
		}
	}
	return out.Bytes(), nil
}

// Fingerprint hashes the frames and their timing alone.
func Fingerprint(data []byte) ([]byte, error) {
	f, err := parse(data)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write(f.head)
	for _, b := range f.blocks {
		if b.kind != blockExtension || b.label == labelControl || b.label == labelText {
			h.Write(b.raw)
		}
	}
	return h.Sum(nil), nil
}
