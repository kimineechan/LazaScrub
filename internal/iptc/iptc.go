// Package iptc reads Photoshop image resources and the IPTC caption data
// stored inside them.
package iptc

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"lazascrub/internal/meta"
)

type dataset struct {
	name string
	risk meta.Risk
}

const notePress = "Caption and credit data in the format used by news agencies."

// Application record (record 2) datasets.
var datasets = map[byte]dataset{
	0:   {"Record version", meta.Low},
	3:   {"Object type", meta.Low},
	4:   {"Object attribute", meta.Low},
	5:   {"Title", meta.Medium},
	7:   {"Edit status", meta.Low},
	10:  {"Urgency", meta.Low},
	12:  {"Subject reference", meta.Medium},
	15:  {"Category", meta.Medium},
	20:  {"Supplemental category", meta.Medium},
	22:  {"Fixture identifier", meta.Medium},
	25:  {"Keywords", meta.Medium},
	26:  {"Location code", meta.High},
	27:  {"Location name", meta.High},
	30:  {"Release date", meta.Medium},
	35:  {"Release time", meta.Medium},
	40:  {"Special instructions", meta.Medium},
	55:  {"Date created", meta.Medium},
	60:  {"Time created", meta.Medium},
	62:  {"Digital creation date", meta.Medium},
	63:  {"Digital creation time", meta.Medium},
	65:  {"Originating program", meta.Medium},
	70:  {"Program version", meta.Medium},
	80:  {"By-line (author)", meta.High},
	85:  {"By-line title", meta.High},
	90:  {"City", meta.High},
	92:  {"Sublocation", meta.High},
	95:  {"Province or state", meta.High},
	100: {"Country code", meta.High},
	101: {"Country", meta.High},
	103: {"Transmission reference", meta.Medium},
	105: {"Headline", meta.Medium},
	110: {"Credit", meta.High},
	115: {"Source", meta.High},
	116: {"Copyright notice", meta.High},
	118: {"Contact", meta.High},
	120: {"Caption", meta.Medium},
	122: {"Caption writer", meta.High},
}

// Photoshop resource blocks that are worth naming in a report.
var resources = map[uint16]string{
	0x03ED: "Resolution",
	0x0404: "IPTC data",
	0x0406: "JPEG quality",
	0x0408: "Grid and guides",
	0x0409: "Thumbnail",
	0x040A: "Copyright flag",
	0x040B: "Web address",
	0x040C: "Thumbnail",
	0x040D: "Global lighting angle",
	0x040F: "Colour profile",
	0x0414: "Document ID seed",
	0x0419: "Global altitude",
	0x041A: "Slices",
	0x041E: "Web address list",
	0x0421: "Version information",
	0x0422: "EXIF data",
	0x0423: "EXIF data",
	0x0424: "XMP packet",
	0x0425: "Caption digest",
	0x0426: "Print scale",
	0x0428: "Pixel aspect ratio",
	0x0BB7: "Clipping path name",
	0x2710: "Print flags",
}

// Parse reads a Photoshop image resource block (the body of a JPEG APP13 segment).
func Parse(data []byte) []meta.Finding {
	var out meta.List
	for len(data) >= 12 && string(data[:4]) == "8BIM" {
		id := uint16(data[4])<<8 | uint16(data[5])
		nameLen := int(data[6]) + 1
		if nameLen%2 == 1 {
			nameLen++
		}
		at := 6 + nameLen
		if at+4 > len(data) {
			break
		}
		size := int(data[at])<<24 | int(data[at+1])<<16 | int(data[at+2])<<8 | int(data[at+3])
		at += 4
		if size < 0 || at+size > len(data) {
			break
		}
		body := data[at : at+size]
		switch id {
		case 0x0404:
			out = append(out, Records(body)...)
		case 0x0409, 0x040C:
			out.Add("Photoshop", "Thumbnail", meta.Size(int64(size))+" preview image", meta.High, meta.NotePreview)
		case 0x040B, 0x041E:
			out.Add("Photoshop", resources[id], meta.Text(body), meta.Medium, "")
		default:
			name, ok := resources[id]
			if !ok {
				name = fmt.Sprintf("Resource 0x%04X", id)
			}
			out.Add("Photoshop", name, meta.Size(int64(size)), meta.Low, "Editing data written by Adobe Photoshop or a compatible app.")
		}
		if size%2 == 1 {
			size++
		}
		if at+size > len(data) {
			break
		}
		data = data[at+size:]
	}
	return out
}

// Records reads raw IPTC datasets.
func Records(data []byte) []meta.Finding {
	var out meta.List
	utf := false
	for len(data) >= 5 && data[0] == 0x1C {
		record, number := data[1], data[2]
		size := int(data[3])<<8 | int(data[4])
		data = data[5:]
		if size&0x8000 != 0 {
			// Extended form: the low bits give the number of length bytes that follow.
			n := size & 0x7FFF
			if n > 4 || n > len(data) {
				break
			}
			size = 0
			for _, b := range data[:n] {
				size = size<<8 | int(b)
			}
			data = data[n:]
		}
		if size < 0 || size > len(data) {
			break
		}
		value := data[:size]
		data = data[size:]
		switch {
		case record == 1 && number == 90:
			utf = bytes.Equal(value, []byte{0x1B, '%', 'G'})
		case record == 2:
			d, ok := datasets[number]
			if !ok {
				d = dataset{fmt.Sprintf("Dataset 2:%d", number), meta.Low}
			}
			text := decode(value, utf)
			if number == 0 || text == "" {
				continue
			}
			out.Add("IPTC", d.name, text, d.risk, notePress)
		}
	}
	return out
}

func decode(b []byte, utf bool) string {
	if utf || utf8.Valid(b) {
		return meta.Line(string(b))
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return meta.Line(string(r))
}
