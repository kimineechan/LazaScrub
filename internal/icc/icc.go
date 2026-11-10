// Package icc summarises ICC colour profiles.
package icc

import (
	"encoding/binary"
	"fmt"

	"lazascrub/internal/meta"
)

const keep = "Kept because removing it would change how the colours are displayed."

// Parse describes a colour profile. Cleaning keeps profiles, so every finding is marked as kept.
func Parse(data []byte) []meta.Finding {
	var out meta.List
	if len(data) < 132 || string(data[36:40]) != "acsp" {
		out.Keep("Colour profile", "Profile", meta.Size(int64(len(data))), meta.Low, keep)
		return out
	}
	if name := tagText(data, "desc"); name != "" {
		out.Keep("Colour profile", "Profile name", name, meta.Low, keep)
	} else {
		out.Keep("Colour profile", "Profile", meta.Size(int64(len(data))), meta.Low, keep)
	}
	if text := tagText(data, "cprt"); text != "" {
		out.Keep("Colour profile", "Profile copyright", text, meta.Low, keep)
	}
	be := binary.BigEndian
	if year := be.Uint16(data[24:]); year != 0 {
		out.Keep("Colour profile", "Profile created", fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d", year,
			be.Uint16(data[26:]), be.Uint16(data[28:]), be.Uint16(data[30:]), be.Uint16(data[32:]), be.Uint16(data[34:])),
			meta.Low, "The date the profile itself was made, not the picture. "+keep)
	}
	return out
}

// tagText reads a text tag in either the version 2 or version 4 encoding.
func tagText(data []byte, signature string) string {
	be := binary.BigEndian
	count := int(be.Uint32(data[128:]))
	for i := 0; i < count && 132+12*i+12 <= len(data); i++ {
		entry := data[132+12*i:]
		if string(entry[:4]) != signature {
			continue
		}
		start, size := uint64(be.Uint32(entry[4:])), uint64(be.Uint32(entry[8:]))
		if size < 12 || start+size > uint64(len(data)) {
			return ""
		}
		body := data[start : start+size]
		switch string(body[:4]) {
		case "desc":
			n := uint64(be.Uint32(body[8:]))
			if 12+n > uint64(len(body)) {
				return ""
			}
			return meta.Text(trim(body[12 : 12+n]))
		case "text":
			return meta.Text(trim(body[8:]))
		case "mluc":
			if len(body) < 28 || be.Uint32(body[8:]) == 0 {
				return ""
			}
			length, offset := uint64(be.Uint32(body[20:])), uint64(be.Uint32(body[24:]))
			if offset+length > uint64(len(body)) {
				return ""
			}
			return meta.Line(meta.UTF16(body[offset:offset+length], true))
		}
		return ""
	}
	return ""
}

func trim(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}
