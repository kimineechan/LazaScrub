// Package webp finds and removes metadata in WebP files.
package webp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"lazascrub/internal/exif"
	"lazascrub/internal/icc"
	"lazascrub/internal/meta"
	"lazascrub/internal/xmp"
)

const (
	flagXMP  = 0x04
	flagEXIF = 0x08
)

type chunk struct {
	kind string
	data []byte
}

// Chunks that hold the picture.
var picture = map[string]bool{"VP8X": true, "VP8 ": true, "VP8L": true, "ALPH": true, "ANIM": true, "ANMF": true, "ICCP": true}

// Is reports whether data starts like a WebP file.
func Is(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func parse(data []byte) ([]chunk, []byte, error) {
	if !Is(data) {
		return nil, nil, errors.New("not a WebP file")
	}
	end := uint64(binary.LittleEndian.Uint32(data[4:])) + 8
	if end > uint64(len(data)) {
		end = uint64(len(data))
	}
	var chunks []chunk
	pos := uint64(12)
	for pos+8 <= end {
		size := uint64(binary.LittleEndian.Uint32(data[pos+4:]))
		if pos+8+size > end {
			return nil, nil, errors.New("the WebP file is damaged: a chunk runs past the end")
		}
		chunks = append(chunks, chunk{kind: string(data[pos : pos+4]), data: data[pos+8 : pos+8+size]})
		pos += 8 + size + size%2
	}
	return chunks, data[end:], nil
}

func tiff(c chunk) []byte { return bytes.TrimPrefix(c.data, []byte("Exif\x00\x00")) }

// Scan lists the metadata in a WebP file.
func Scan(data []byte) ([]meta.Finding, error) {
	chunks, trailing, err := parse(data)
	if err != nil {
		return nil, err
	}
	var out meta.List
	for _, c := range chunks {
		switch {
		case c.kind == "EXIF":
			info, err := exif.Parse(tiff(c))
			if err != nil {
				out.Add("EXIF", "EXIF block", meta.Size(int64(len(c.data)))+" that could not be read", meta.Medium, "")
			}
			out = append(out, info.Findings...)
		case c.kind == "XMP ":
			out = append(out, xmp.Parse(c.data, "XMP")...)
		case c.kind == "ICCP":
			out = append(out, icc.Parse(c.data)...)
		case !picture[c.kind]:
			out.Add("WebP", "Chunk "+meta.Text([]byte(c.kind)), meta.Size(int64(len(c.data))), meta.Medium, meta.NoteExtra)
		}
	}
	out.Hidden(trailing)
	return out, nil
}

// Clean returns a copy of the file without its metadata. Picture data, the
// colour profile and the display orientation are carried over unchanged.
func Clean(data []byte) ([]byte, error) {
	chunks, _, err := parse(data)
	if err != nil {
		return nil, err
	}
	var kept []chunk
	header := -1
	hasEXIF := false
	for _, c := range chunks {
		switch {
		case c.kind == "EXIF":
			if info, err := exif.Parse(tiff(c)); err == nil && info.Orientation >= 2 && info.Orientation <= 8 {
				kept = append(kept, chunk{"EXIF", exif.Orientation(info.Orientation)})
				hasEXIF = true
			}
		case picture[c.kind]:
			if c.kind == "VP8X" && len(c.data) >= 10 {
				header = len(kept)
				c.data = append([]byte(nil), c.data...)
			}
			kept = append(kept, c)
		}
	}
	if header >= 0 {
		flags := kept[header].data[0] &^ (flagXMP | flagEXIF)
		if hasEXIF {
			flags |= flagEXIF
		}
		kept[header].data[0] = flags
	} else if hasEXIF {
		// Only the extended layout may carry an EXIF chunk.
		plain := kept[:0]
		for _, c := range kept {
			if c.kind != "EXIF" {
				plain = append(plain, c)
			}
		}
		kept = plain
	}
	out := bytes.NewBufferString("RIFF\x00\x00\x00\x00WEBP")
	for _, c := range kept {
		out.WriteString(c.kind)
		binary.Write(out, binary.LittleEndian, uint32(len(c.data)))
		out.Write(c.data)
		if len(c.data)%2 == 1 {
			out.WriteByte(0)
		}
	}
	result := out.Bytes()
	binary.LittleEndian.PutUint32(result[4:], uint32(len(result)-8))
	return result, nil
}

// Fingerprint hashes the picture data alone.
func Fingerprint(data []byte) ([]byte, error) {
	chunks, _, err := parse(data)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	for _, c := range chunks {
		switch c.kind {
		case "VP8 ", "VP8L", "ALPH", "ANIM", "ANMF":
			h.Write([]byte(c.kind))
			h.Write(c.data)
		}
	}
	return h.Sum(nil), nil
}
