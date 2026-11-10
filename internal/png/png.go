// Package png finds and removes metadata in PNG files.
package png

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"strings"

	"lazascrub/internal/exif"
	"lazascrub/internal/icc"
	"lazascrub/internal/iptc"
	"lazascrub/internal/meta"
	"lazascrub/internal/xmp"
)

const signature = "\x89PNG\r\n\x1a\n"

type chunk struct {
	kind string
	data []byte
	raw  []byte // length, type, data and checksum as stored
}

// Chunks that hold the picture or control how it is drawn.
var picture = map[string]bool{
	"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true, "tRNS": true, "gAMA": true, "cHRM": true,
	"sRGB": true, "iCCP": true, "sBIT": true, "bKGD": true, "hIST": true, "pHYs": true, "sPLT": true,
	"cICP": true, "mDCV": true, "cLLI": true, "acTL": true, "fcTL": true, "fdAT": true, "oFFs": true,
	"pCAL": true, "sCAL": true, "sTER": true,
}

type rating struct {
	risk meta.Risk
	note string
}

const noteAI = "The prompt and settings an AI image generator used to make this picture."

// Text keywords with a known meaning.
var keywords = map[string]rating{
	"Author":            {meta.High, "Names a person."},
	"Copyright":         {meta.High, "Names the owner of the picture."},
	"Software":          {meta.Medium, "Names the app that wrote the file."},
	"Creation Time":     {meta.Medium, "Shows when the picture was made."},
	"parameters":        {meta.Medium, noteAI},
	"prompt":            {meta.Medium, noteAI},
	"workflow":          {meta.Medium, noteAI},
	"Dream":             {meta.Medium, noteAI},
	"sd-metadata":       {meta.Medium, noteAI},
	"invokeai_metadata": {meta.Medium, noteAI},
	"generation_data":   {meta.Medium, noteAI},
}

// Is reports whether data starts like a PNG file.
func Is(data []byte) bool { return bytes.HasPrefix(data, []byte(signature)) }

func parse(data []byte) ([]chunk, []byte, error) {
	if !Is(data) {
		return nil, nil, errors.New("not a PNG file")
	}
	var chunks []chunk
	pos := len(signature)
	for pos < len(data) {
		if pos+12 > len(data) {
			break
		}
		size := uint64(binary.BigEndian.Uint32(data[pos:]))
		end := uint64(pos) + 12 + size
		if end > uint64(len(data)) {
			break
		}
		c := chunk{kind: string(data[pos+4 : pos+8]), data: data[pos+8 : uint64(pos)+8+size], raw: data[pos:end]}
		chunks = append(chunks, c)
		pos = int(end)
		if c.kind == "IEND" {
			return chunks, data[pos:], nil
		}
	}
	// Damaged or cut short. Keep the rest untouched.
	if pos < len(data) {
		chunks = append(chunks, chunk{raw: data[pos:]})
	}
	return chunks, nil, nil
}

func inflate(b []byte) []byte {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	defer r.Close()
	out, _ := io.ReadAll(io.LimitReader(r, 32<<20))
	return out
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// text splits a text chunk into its keyword and its value.
func text(c chunk) (string, []byte, bool) {
	i := bytes.IndexByte(c.data, 0)
	if i < 0 {
		return "", nil, false
	}
	key, rest := latin1(c.data[:i]), c.data[i+1:]
	switch c.kind {
	case "tEXt":
		return key, []byte(latin1(rest)), true
	case "zTXt":
		if len(rest) < 1 {
			return key, nil, true
		}
		return key, []byte(latin1(inflate(rest[1:]))), true
	}
	// iTXt: compression flag, method, language, translated keyword, text.
	if len(rest) < 2 {
		return key, nil, true
	}
	compressed := rest[0] == 1
	rest = rest[2:]
	for n := 0; n < 2; n++ {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			return key, nil, true
		}
		rest = rest[j+1:]
	}
	if compressed {
		rest = inflate(rest)
	}
	return key, rest, true
}

// rawProfile decodes the hex-encoded metadata blocks that ImageMagick writes
// as text chunks named "Raw profile type ...".
func rawProfile(value []byte) []byte {
	lines := strings.SplitN(strings.TrimLeft(string(value), "\n"), "\n", 3)
	if len(lines) < 3 {
		return nil
	}
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, lines[2])
	out, err := hex.DecodeString(clean)
	if err != nil {
		return nil
	}
	return out
}

// Scan lists the metadata in a PNG file.
func Scan(data []byte) ([]meta.Finding, error) {
	chunks, trailing, err := parse(data)
	if err != nil {
		return nil, err
	}
	var out meta.List
	for _, c := range chunks {
		switch c.kind {
		case "tEXt", "zTXt", "iTXt":
			key, value, ok := text(c)
			if !ok {
				continue
			}
			lower := strings.ToLower(key)
			switch {
			case key == "XML:com.adobe.xmp":
				out = append(out, xmp.Parse(value, "XMP")...)
			case strings.HasPrefix(lower, "raw profile type"):
				block := rawProfile(value)
				switch kind := strings.TrimSpace(lower[len("raw profile type"):]); {
				case block == nil:
					out.Add("Text", key, meta.Size(int64(len(value))), meta.Medium, "An encoded metadata block.")
				case kind == "exif" || kind == "app1":
					block = bytes.TrimPrefix(block, []byte("Exif\x00\x00"))
					if info, err := exif.Parse(block); err == nil {
						out = append(out, info.Findings...)
					}
				case kind == "iptc" || kind == "8bim":
					if bytes.HasPrefix(block, []byte("8BIM")) {
						out = append(out, iptc.Parse(block)...)
					} else {
						out = append(out, iptc.Records(block)...)
					}
				case kind == "xmp":
					out = append(out, xmp.Parse(block, "XMP")...)
				default:
					out.Add("Text", key, meta.Size(int64(len(block))), meta.Medium, "An encoded metadata block.")
				}
			default:
				shown := meta.Text(value)
				r, known := keywords[key]
				if !known {
					r = rating{max(meta.Rate(key, shown), meta.Medium), "Text stored by the app that saved the file."}
				}
				if n := []rune(shown); len(n) > 2000 {
					shown = fmt.Sprintf("%s... (%d characters)", string(n[:600]), len(n))
				}
				out.Add("Text", key, shown, r.risk, r.note)
			}
		case "eXIf":
			info, err := exif.Parse(c.data)
			if err != nil {
				out.Add("EXIF", "EXIF block", meta.Size(int64(len(c.data)))+" that could not be read", meta.Medium, "")
			}
			out = append(out, info.Findings...)
		case "tIME":
			if len(c.data) == 7 {
				out.Add("PNG", "Last modified", fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d UTC",
					binary.BigEndian.Uint16(c.data), c.data[2], c.data[3], c.data[4], c.data[5], c.data[6]),
					meta.Medium, "Shows when the picture was last changed.")
			}
		case "pHYs":
			if len(c.data) == 9 {
				x, y := binary.BigEndian.Uint32(c.data), binary.BigEndian.Uint32(c.data[4:])
				value := fmt.Sprintf("%d x %d (aspect ratio)", x, y)
				if c.data[8] == 1 {
					value = fmt.Sprintf("%d x %d dpi", int(math.Round(float64(x)*0.0254)), int(math.Round(float64(y)*0.0254)))
				}
				out.Keep("PNG", "Resolution", value, meta.Low, meta.NoteNeeded)
			}
		case "iCCP":
			if i := bytes.IndexByte(c.data, 0); i >= 0 && i+2 <= len(c.data) {
				out = append(out, icc.Parse(inflate(c.data[i+2:]))...)
			}
		case "":
		default:
			if picture[c.kind] || c.kind[0] < 'a' {
				continue
			}
			note := meta.NoteExtra
			if c.kind == "caBX" {
				note = "Content credentials, which can record the tools or AI model that made the picture."
			}
			out.Add("PNG", "Private chunk "+c.kind, meta.Size(int64(len(c.data))), meta.Medium, note)
		}
	}
	out.Hidden(trailing)
	return out, nil
}

func build(kind string, data []byte) []byte {
	out := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(out, uint32(len(data)))
	copy(out[4:], kind)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[4:]))
}

// Clean returns a copy of the file without its metadata. Picture data and
// everything that controls how it is drawn are carried over unchanged.
func Clean(data []byte) ([]byte, error) {
	chunks, _, err := parse(data)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(len(data))
	out.WriteString(signature)
	for _, c := range chunks {
		switch {
		case c.kind == "" || picture[c.kind] || c.kind[0] < 'a':
			out.Write(c.raw)
		case c.kind == "eXIf":
			if info, err := exif.Parse(c.data); err == nil && info.Orientation >= 2 && info.Orientation <= 8 {
				out.Write(build("eXIf", exif.Orientation(info.Orientation)))
			}
		}
	}
	return out.Bytes(), nil
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
		case "IHDR", "PLTE", "tRNS", "IDAT", "acTL", "fcTL", "fdAT", "":
			h.Write(c.raw)
		}
	}
	return h.Sum(nil), nil
}
