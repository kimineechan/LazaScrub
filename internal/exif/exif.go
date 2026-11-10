// Package exif reads EXIF data, which is stored as a TIFF structure.
package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"lazascrub/internal/meta"
)

// Info is the result of reading one EXIF block.
type Info struct {
	Findings    []meta.Finding
	Orientation int    // 1 to 8, or 0 when the tag is absent
	Thumbnail   []byte // embedded preview image, if any
}

const (
	dirImage = iota
	dirPhoto
	dirGPS
	dirInterop
	dirThumb
)

var sections = [...]string{"EXIF", "EXIF", "GPS", "EXIF", "Thumbnail"}

// Byte sizes of the TIFF field types 1 to 13.
var typeSize = [...]uint64{0, 1, 1, 2, 4, 8, 1, 1, 2, 4, 8, 4, 8, 4}

type entry struct {
	tag   uint16
	typ   uint16
	count uint32
	raw   []byte
}

type reader struct {
	data  []byte
	order binary.ByteOrder
	seen  map[uint32]bool
	out   meta.List
	info  Info
	maker string
	thumb [2]uint32 // offset and length of the preview image
}

// Parse reads a TIFF block: the EXIF payload without its "Exif\0\0" prefix.
func Parse(data []byte) (Info, error) {
	if len(data) < 8 {
		return Info{}, errors.New("the EXIF block is too short")
	}
	r := &reader{data: data, seen: map[uint32]bool{}}
	switch string(data[:2]) {
	case "II":
		r.order = binary.LittleEndian
	case "MM":
		r.order = binary.BigEndian
	default:
		return Info{}, errors.New("the EXIF block has no TIFF header")
	}
	if r.order.Uint16(data[2:]) != 42 {
		return Info{}, errors.New("the EXIF block has an unknown TIFF version")
	}
	next := r.dir(r.order.Uint32(data[4:]), dirImage)
	if next != 0 {
		r.dir(next, dirThumb)
	}
	r.preview()
	r.info.Findings = r.out
	return r.info, nil
}

// dir reads one image file directory and returns the offset of the next one.
func (r *reader) dir(offset uint32, kind int) uint32 {
	if offset == 0 || r.seen[offset] || uint64(offset)+2 > uint64(len(r.data)) {
		return 0
	}
	r.seen[offset] = true
	n := int(r.order.Uint16(r.data[offset:]))
	if n > 1024 {
		n = 1024
	}
	base := uint64(offset) + 2
	gps := map[uint16]entry{}
	for i := 0; i < n; i++ {
		at := base + uint64(i)*12
		if at+12 > uint64(len(r.data)) {
			return 0
		}
		raw := r.data[at : at+12]
		e := entry{tag: r.order.Uint16(raw), typ: r.order.Uint16(raw[2:]), count: r.order.Uint32(raw[4:])}
		if e.typ == 0 || int(e.typ) >= len(typeSize) {
			continue
		}
		size := uint64(e.count) * typeSize[e.typ]
		if size <= 4 {
			e.raw = raw[8 : 8+size]
		} else {
			start := uint64(r.order.Uint32(raw[8:]))
			if start+size > uint64(len(r.data)) {
				continue
			}
			e.raw = r.data[start : start+size]
		}
		switch {
		case kind == dirImage && e.tag == 0x8769:
			r.dir(r.long(e), dirPhoto)
		case kind == dirImage && e.tag == 0x8825:
			r.dir(r.long(e), dirGPS)
		case kind == dirPhoto && e.tag == 0xA005:
			r.dir(r.long(e), dirInterop)
		case kind == dirGPS:
			gps[e.tag] = e
		default:
			r.field(e, kind)
		}
	}
	if kind == dirGPS {
		r.position(gps)
	}
	end := base + uint64(n)*12
	if end+4 > uint64(len(r.data)) {
		return 0
	}
	return r.order.Uint32(r.data[end:])
}

func (r *reader) field(e entry, kind int) {
	section := sections[kind]
	table := imageTags
	switch kind {
	case dirPhoto:
		table = photoTags
	case dirInterop:
		table = interopTags
	}
	t, known := table[e.tag]
	if !known && kind != dirInterop {
		t, known = photoTags[e.tag]
		if !known {
			t, known = imageTags[e.tag]
		}
	}
	if !known {
		r.out.Add(section, fmt.Sprintf("Tag 0x%04X", e.tag), r.plain(e), meta.Low, "A tag this version does not name.")
		return
	}
	switch e.tag {
	case 0x0112:
		if kind == dirImage {
			r.info.Orientation = int(r.long(e))
			if r.info.Orientation >= 2 && r.info.Orientation <= 8 {
				r.out.Keep(section, t.name, r.format(e), meta.Low, "Kept so the picture stays the right way up.")
				return
			}
		}
	case 0x010F:
		if kind == dirImage {
			r.maker = r.text(e)
		}
	case 0x0201:
		if kind == dirThumb {
			r.thumb[0] = r.long(e)
			return
		}
	case 0x0202:
		if kind == dirThumb {
			r.thumb[1] = r.long(e)
			return
		}
	case 0x927C:
		r.out.Add(section, t.name, r.makerNote(e), t.risk, t.note)
		return
	}
	risk := t.risk
	if kind == dirThumb && risk > meta.Low {
		risk = meta.Low
	}
	r.out.Add(section, t.name, r.format(e), risk, t.note)
}

// preview records the small copy of the picture stored in the second directory.
func (r *reader) preview() {
	start, size := uint64(r.thumb[0]), uint64(r.thumb[1])
	if start == 0 || size == 0 || start+size > uint64(len(r.data)) {
		return
	}
	r.info.Thumbnail = r.data[start : start+size]
	value := "JPEG, " + meta.Size(int64(size))
	if w, h := jpegSize(r.info.Thumbnail); w > 0 {
		value = fmt.Sprintf("%d x %d JPEG, %s", w, h, meta.Size(int64(size)))
	}
	r.out.Add("Thumbnail", "Preview image", value, meta.High, meta.NotePreview)
}

// jpegSize finds the pixel size of a JPEG without decoding it.
func jpegSize(b []byte) (int, int) {
	for i := 2; i+9 < len(b); {
		if b[i] != 0xFF {
			return 0, 0
		}
		m := b[i+1]
		if m == 0xFF {
			i++
			continue
		}
		size := int(b[i+2])<<8 | int(b[i+3])
		if m >= 0xC0 && m <= 0xCF && m != 0xC4 && m != 0xC8 && m != 0xCC {
			return int(b[i+7])<<8 | int(b[i+8]), int(b[i+5])<<8 | int(b[i+6])
		}
		if m == 0xDA || size < 2 {
			return 0, 0
		}
		i += 2 + size
	}
	return 0, 0
}

func (r *reader) long(e entry) uint32 {
	if v := r.uints(e); len(v) > 0 {
		return uint32(v[0])
	}
	return 0
}

func (r *reader) uints(e entry) []uint64 {
	var out []uint64
	switch e.typ {
	case 1, 7:
		for _, b := range e.raw {
			out = append(out, uint64(b))
		}
	case 3:
		for i := 0; i+2 <= len(e.raw); i += 2 {
			out = append(out, uint64(r.order.Uint16(e.raw[i:])))
		}
	case 4, 13:
		for i := 0; i+4 <= len(e.raw); i += 4 {
			out = append(out, uint64(r.order.Uint32(e.raw[i:])))
		}
	}
	return out
}

// floats returns every value of a numeric entry as a float.
func (r *reader) floats(e entry) []float64 {
	var out []float64
	switch e.typ {
	case 1, 3, 4, 7, 13:
		for _, v := range r.uints(e) {
			out = append(out, float64(v))
		}
	case 6:
		for _, b := range e.raw {
			out = append(out, float64(int8(b)))
		}
	case 8:
		for i := 0; i+2 <= len(e.raw); i += 2 {
			out = append(out, float64(int16(r.order.Uint16(e.raw[i:]))))
		}
	case 9:
		for i := 0; i+4 <= len(e.raw); i += 4 {
			out = append(out, float64(int32(r.order.Uint32(e.raw[i:]))))
		}
	case 5:
		for i := 0; i+8 <= len(e.raw); i += 8 {
			out = append(out, ratio(float64(r.order.Uint32(e.raw[i:])), float64(r.order.Uint32(e.raw[i+4:]))))
		}
	case 10:
		for i := 0; i+8 <= len(e.raw); i += 8 {
			out = append(out, ratio(float64(int32(r.order.Uint32(e.raw[i:]))), float64(int32(r.order.Uint32(e.raw[i+4:])))))
		}
	case 11:
		for i := 0; i+4 <= len(e.raw); i += 4 {
			out = append(out, float64(math.Float32frombits(r.order.Uint32(e.raw[i:]))))
		}
	case 12:
		for i := 0; i+8 <= len(e.raw); i += 8 {
			out = append(out, math.Float64frombits(r.order.Uint64(e.raw[i:])))
		}
	}
	return out
}

func ratio(n, d float64) float64 {
	if d == 0 {
		return 0
	}
	return n / d
}

func (r *reader) text(e entry) string {
	b := e.raw
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	return meta.Text(b)
}

// num prints a float without trailing zeros.
func num(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	s := fmt.Sprintf("%.4f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// plain prints an entry with no knowledge of what its tag means.
func (r *reader) plain(e entry) string {
	if e.typ == 2 {
		return r.text(e)
	}
	if (e.typ == 1 || e.typ == 7) && len(e.raw) > 16 {
		return fmt.Sprintf("%d bytes", len(e.raw))
	}
	values := r.floats(e)
	parts := make([]string, 0, len(values))
	for i, v := range values {
		if i == 16 {
			parts = append(parts, fmt.Sprintf("and %d more", len(values)-16))
			break
		}
		parts = append(parts, num(v))
	}
	return strings.Join(parts, ", ")
}

// charset decodes the text types that start with an eight-byte character code.
func charset(b []byte) string {
	if len(b) < 8 {
		return meta.Text(b)
	}
	body := b[8:]
	switch string(b[:8]) {
	case "UNICODE\x00":
		// Writers disagree on the byte order, so pick the one that reads as text.
		le, be := meta.UTF16(body, false), meta.UTF16(body, true)
		if asciiShare(be) > asciiShare(le) {
			return meta.Line(be)
		}
		return meta.Line(le)
	case "ASCII\x00\x00\x00", "\x00\x00\x00\x00\x00\x00\x00\x00", "JIS\x00\x00\x00\x00\x00":
		return meta.Text(bytes.TrimRight(body, "\x00 "))
	}
	return meta.Text(b)
}

func asciiShare(s string) int {
	n := 0
	for _, c := range s {
		if c >= 0x20 && c < 0x7F {
			n++
		}
	}
	return n
}

func (r *reader) makerNote(e entry) string {
	vendor := ""
	head := string(e.raw[:min(len(e.raw), 12)])
	for _, v := range []struct{ sig, name string }{
		{"Apple iOS", "Apple"}, {"Nikon", "Nikon"}, {"OLYMP", "Olympus"}, {"OM SYSTEM", "OM System"},
		{"FUJIFILM", "Fujifilm"}, {"SONY", "Sony"}, {"Panasonic", "Panasonic"}, {"AOC", "Pentax"},
		{"PENTAX", "Pentax"}, {"SIGMA", "Sigma"}, {"FOVEON", "Sigma"}, {"LEICA", "Leica"},
		{"Ricoh", "Ricoh"}, {"RICOH", "Ricoh"}, {"SAMSUNG", "Samsung"}, {"QVC", "Casio"},
		{"MOT", "Motorola"}, {"HUAWEI", "Huawei"}, {"DJI", "DJI"}, {"GOOGLE", "Google"},
	} {
		if strings.HasPrefix(head, v.sig) {
			vendor = v.name
			break
		}
	}
	if vendor == "" && r.maker != "" {
		vendor = r.maker
	}
	if vendor == "" {
		return fmt.Sprintf("%s of manufacturer data", meta.Size(int64(len(e.raw))))
	}
	return fmt.Sprintf("%s of %s data", meta.Size(int64(len(e.raw))), vendor)
}

// format prints an entry using what is known about its tag.
func (r *reader) format(e entry) string {
	v := r.floats(e)
	first := 0.0
	if len(v) > 0 {
		first = v[0]
	}
	if names, ok := enums[e.tag]; ok && len(v) == 1 {
		if name, ok := names[int(first)]; ok {
			return name
		}
		return num(first)
	}
	switch e.tag {
	case 0x0132, 0x9003, 0x9004: // dates are stored as YYYY:MM:DD HH:MM:SS
		s := r.text(e)
		if len(s) >= 10 && s[4] == ':' && s[7] == ':' {
			s = s[:4] + "-" + s[5:7] + "-" + s[8:]
		}
		return s
	case 0x829A:
		return shutter(first)
	case 0x9201:
		return shutter(math.Pow(2, -first))
	case 0x829D:
		return "f/" + num(round(first, 1))
	case 0x9202, 0x9205:
		return "f/" + num(round(math.Pow(math.Sqrt2, first), 1))
	case 0x9203, 0x9204:
		return fmt.Sprintf("%+.2f EV", first)
	case 0x920A:
		return num(round(first, 1)) + " mm"
	case 0xA405:
		return num(first) + " mm (35 mm equivalent)"
	case 0x9206:
		return num(round(first, 2)) + " m"
	case 0x9000, 0xA000, 0x0002:
		if s := string(e.raw); len(s) == 4 && e.typ == 7 {
			return strings.TrimLeft(s[:2], "0") + "." + s[2:]
		}
	case 0x9209:
		return flash(int(first))
	case 0x9286:
		return charset(e.raw)
	case 0x9C9B, 0x9C9C, 0x9C9D, 0x9C9E, 0x9C9F:
		return meta.Line(meta.UTF16(e.raw, false))
	case 0x9101:
		parts := []string{}
		for _, c := range e.raw {
			parts = append(parts, [...]string{"-", "Y", "Cb", "Cr", "R", "G", "B"}[min(int(c), 6)])
		}
		return strings.Join(parts, ", ")
	case 0xA300:
		if first == 3 {
			return "Digital camera"
		}
	case 0xA301:
		if first == 1 {
			return "Directly photographed"
		}
	case 0xA432:
		if len(v) == 4 {
			focal := num(v[0])
			if v[1] != v[0] {
				focal += "-" + num(v[1])
			}
			aperture := num(v[2])
			if v[3] != v[2] && v[3] != 0 {
				aperture += "-" + num(v[3])
			}
			return focal + " mm f/" + aperture
		}
	case 0x02BC, 0x83BB, 0x8649, 0x8773, 0xC4A5, 0xEA1C, 0xA302:
		return fmt.Sprintf("%d bytes", len(e.raw))
	}
	return r.plain(e)
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

func shutter(seconds float64) string {
	if seconds <= 0 {
		return "0 s"
	}
	if seconds < 1 {
		return fmt.Sprintf("1/%d s", int(math.Round(1/seconds)))
	}
	return num(round(seconds, 1)) + " s"
}

func flash(v int) string {
	s := "Did not fire"
	if v&1 == 1 {
		s = "Fired"
	}
	switch (v >> 3) & 3 {
	case 1:
		s += ", forced on"
	case 2:
		s += ", forced off"
	case 3:
		s += ", auto"
	}
	if v&0x40 != 0 {
		s += ", red-eye reduction"
	}
	return s
}

// position turns the GPS directory into readable findings.
func (r *reader) position(g map[uint16]entry) {
	const note = "Shows where the picture was taken."
	ref := func(tag uint16) string {
		if e, ok := g[tag]; ok {
			return strings.ToUpper(r.text(e))
		}
		return ""
	}
	degrees := func(tag uint16) (float64, bool) {
		e, ok := g[tag]
		if !ok {
			return 0, false
		}
		v := r.floats(e)
		if len(v) != 3 {
			return 0, false
		}
		return v[0] + v[1]/60 + v[2]/3600, true
	}
	lat, okLat := degrees(2)
	lon, okLon := degrees(4)
	if okLat && okLon {
		if ref(1) == "S" {
			lat = -lat
		}
		if ref(3) == "W" {
			lon = -lon
		}
		r.out.Add("GPS", "Position", fmt.Sprintf("%.6f, %.6f", lat, lon), meta.High,
			"Latitude and longitude of the place the picture was taken. Paste it into a map to see the spot.")
	}
	if e, ok := g[6]; ok {
		v := r.floats(e)
		if len(v) == 1 {
			where := "above sea level"
			if e2, ok := g[5]; ok && r.long(e2) == 1 {
				where = "below sea level"
			}
			r.out.Add("GPS", "Altitude", num(round(v[0], 1))+" m "+where, meta.High, note)
		}
	}
	if e, ok := g[7]; ok {
		if v := r.floats(e); len(v) == 3 {
			stamp := fmt.Sprintf("%02d:%02d:%02d UTC", int(v[0]), int(v[1]), int(v[2]))
			if date := ref(29); date != "" {
				stamp = strings.ReplaceAll(date, ":", "-") + " " + stamp
			}
			r.out.Add("GPS", "Satellite time", stamp, meta.High, "The exact moment the position was fixed.")
		}
	} else if date := ref(29); date != "" {
		r.out.Add("GPS", "Satellite date", strings.ReplaceAll(date, ":", "-"), meta.High, note)
	}
	pairs := []struct {
		value, ref uint16
		name       string
		units      map[string]string
	}{
		{13, 12, "Speed", map[string]string{"K": " km/h", "M": " mph", "N": " knots"}},
		{15, 14, "Direction of travel", map[string]string{"T": "° (true north)", "M": "° (magnetic north)"}},
		{17, 16, "Direction the camera faced", map[string]string{"T": "° (true north)", "M": "° (magnetic north)"}},
		{24, 23, "Bearing to destination", map[string]string{"T": "° (true north)", "M": "° (magnetic north)"}},
		{26, 25, "Distance to destination", map[string]string{"K": " km", "M": " miles", "N": " nautical miles"}},
	}
	for _, p := range pairs {
		if e, ok := g[p.value]; ok {
			if v := r.floats(e); len(v) == 1 {
				r.out.Add("GPS", p.name, num(round(v[0], 2))+p.units[ref(p.ref)], meta.High, note)
			}
		}
	}
	destLat, okLat := degrees(20)
	destLon, okLon := degrees(22)
	if okLat && okLon {
		if ref(19) == "S" {
			destLat = -destLat
		}
		if ref(21) == "W" {
			destLon = -destLon
		}
		r.out.Add("GPS", "Destination", fmt.Sprintf("%.6f, %.6f", destLat, destLon), meta.High, note)
	}
	rest := []struct {
		tag  uint16
		name string
	}{
		{0, "GPS version"}, {8, "Satellites"}, {9, "Receiver status"}, {10, "Measure mode"},
		{11, "Precision (DOP)"}, {18, "Map datum"}, {27, "Processing method"}, {28, "Area name"},
		{30, "Differential correction"}, {31, "Position error"},
	}
	for _, t := range rest {
		e, ok := g[t.tag]
		if !ok {
			continue
		}
		value := r.plain(e)
		switch t.tag {
		case 0:
			value = strings.ReplaceAll(value, ", ", ".")
		case 27, 28:
			value = charset(e.raw)
		case 31:
			value += " m"
		}
		r.out.Add("GPS", t.name, value, meta.High, note)
	}
}

// Orientation builds the smallest valid EXIF block that holds only the
// orientation tag, so a cleaned picture is still shown the right way up.
func Orientation(value int) []byte {
	b := make([]byte, 26)
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:], 8)
	binary.LittleEndian.PutUint16(b[8:], 1)
	binary.LittleEndian.PutUint16(b[10:], 0x0112)
	binary.LittleEndian.PutUint16(b[12:], 3)
	binary.LittleEndian.PutUint32(b[14:], 1)
	binary.LittleEndian.PutUint16(b[18:], uint16(value))
	return b
}
