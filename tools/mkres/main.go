// mkres draws the application icon and packs it, the manifest and the
// version information into a resource object for Go's Windows linker. It
// needs nothing but Go, so no resource compiler has to be installed.
//
// Run it from the repository root after changing the icon design, the
// manifest or the VERSION file:
//
//	go run ./tools/mkres
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	iconPath     = "assets/lazascrub.ico"
	manifestPath = "assets/app.manifest"
	objectPath   = "cmd/lazascrub/resources_windows_amd64.syso"
)

var sizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mkres:", err)
		os.Exit(1)
	}
}

func run() error {
	version, err := os.ReadFile("VERSION")
	if err != nil {
		return err
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		images[i] = encode(draw(size))
	}
	if err := os.WriteFile(iconPath, icon(images), 0644); err != nil {
		return err
	}
	info, err := versionInfo(strings.TrimSpace(string(version)))
	if err != nil {
		return err
	}
	if err := os.WriteFile(objectPath, object(images, manifest, info), 0644); err != nil {
		return err
	}
	fmt.Println("Wrote", iconPath, "and", objectPath)
	return nil
}

// distance returns how far a point is outside a rounded rectangle. It is
// negative inside.
func distance(x, y, left, top, width, height, radius float64) float64 {
	dx := math.Abs(x-(left+width/2)) - (width/2 - radius)
	dy := math.Abs(y-(top+height/2)) - (height/2 - radius)
	return math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0) - radius
}

// draw renders the icon on a 96-unit grid: a dark rounded tile holding an
// outlined square, with a small solid square lifted off its corner.
func draw(size int) *image.NRGBA {
	const samples = 6
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	unit := 96 / float64(size)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			tile, mark := 0, 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) * unit
					y := (float64(py) + (float64(sy)+0.5)/samples) * unit
					if distance(x, y, 0, 0, 96, 96, 24) > 0 {
						continue
					}
					tile++
					outline := math.Abs(distance(x, y, 19, 38, 38, 38, 9)) <= 3
					chip := distance(x, y, 63, 16, 18, 18, 5) <= 0
					if outline || chip {
						mark++
					}
				}
			}
			if tile == 0 {
				continue
			}
			white := float64(mark) / float64(tile)
			shade := uint8(math.Round(0x15 + (0xFF-0x15)*white))
			img.SetNRGBA(px, py, color.NRGBA{shade, shade, shade, uint8(math.Round(255 * float64(tile) / (samples * samples)))})
		}
	}
	return img
}

// encode stores one icon image: PNG for the large size, a bitmap for the
// small ones, which is the form every version of Windows reads.
func encode(img *image.NRGBA) []byte {
	size := img.Bounds().Dx()
	var out bytes.Buffer
	if size >= 256 {
		png.Encode(&out, img)
		return out.Bytes()
	}
	binary.Write(&out, binary.LittleEndian, struct {
		Size          uint32
		Width, Height int32
		Planes, Bits  uint16
		Rest          [6]uint32
	}{Size: 40, Width: int32(size), Height: int32(2 * size), Planes: 1, Bits: 32})
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			c := img.NRGBAAt(x, y)
			out.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	out.Write(make([]byte, size*((size+31)/32*4)))
	return out.Bytes()
}

// entry describes one image in an icon directory.
func entry(size, length int) []byte {
	b := make([]byte, 12)
	b[0], b[1] = byte(size), byte(size) // 256 is stored as 0
	binary.LittleEndian.PutUint16(b[4:], 1)
	binary.LittleEndian.PutUint16(b[6:], 32)
	binary.LittleEndian.PutUint32(b[8:], uint32(length))
	return b
}

// icon builds the .ico file.
func icon(images [][]byte) []byte {
	out := []byte{0, 0, 1, 0, byte(len(images)), 0}
	offset := 6 + 16*len(images)
	for i, data := range images {
		out = append(out, entry(sizes[i], len(data))...)
		out = binary.LittleEndian.AppendUint32(out, uint32(offset))
		offset += len(data)
	}
	for _, data := range images {
		out = append(out, data...)
	}
	return out
}

// node builds one block of a version resource. Every block is a length, a
// value length, a type, a UTF-16 key, a value and then its child blocks,
// each aligned to four bytes.
func node(key string, text bool, value []byte, children ...[]byte) []byte {
	pad := func(b []byte) []byte {
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		return b
	}
	valueLength, kind := len(value), 0
	if text {
		valueLength, kind = len(value)/2, 1
	}
	b := []byte{0, 0, byte(valueLength), byte(valueLength >> 8), byte(kind), 0}
	b = pad(append(b, wide(key)...))
	b = append(b, value...)
	for _, child := range children {
		b = append(pad(b), child...)
	}
	binary.LittleEndian.PutUint16(b, uint16(len(b)))
	return b
}

func wide(s string) []byte {
	var out []byte
	for _, unit := range append(utf16.Encode([]rune(s)), 0) {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return out
}

// versionInfo builds the details Explorer shows on the file's Properties page.
func versionInfo(version string) ([]byte, error) {
	var parts [4]uint32
	for i, field := range strings.SplitN(strings.SplitN(version, "-", 2)[0], ".", 3) {
		n, err := strconv.ParseUint(field, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("VERSION must look like 1.0.0, not %q", version)
		}
		parts[i] = uint32(n)
	}
	fixed := make([]byte, 52)
	for i, v := range []uint32{
		0xFEEF04BD, 0x00010000,
		parts[0]<<16 | parts[1], parts[2]<<16 | parts[3],
		parts[0]<<16 | parts[1], parts[2]<<16 | parts[3],
		0x3F, 0, 0x00040004, 1,
	} {
		binary.LittleEndian.PutUint32(fixed[4*i:], v)
	}
	var texts [][]byte
	for _, s := range [][2]string{
		{"CompanyName", "LazaScrub contributors"},
		{"FileDescription", "LazaScrub"},
		{"FileVersion", version},
		{"InternalName", "LazaScrub"},
		{"LegalCopyright", "Copyright (c) 2026 LazaScrub contributors. MIT licence."},
		{"OriginalFilename", "LazaScrub.exe"},
		{"ProductName", "LazaScrub"},
		{"ProductVersion", version},
	} {
		texts = append(texts, node(s[0], true, wide(s[1])))
	}
	return node("VS_VERSION_INFO", false, fixed,
		node("StringFileInfo", true, nil, node("040904B0", true, nil, texts...)),
		node("VarFileInfo", true, nil, node("Translation", false, []byte{0x09, 0x04, 0xB0, 0x04})),
	), nil
}

// object builds an AMD64 COFF file with a single .rsrc section holding the
// icon images, the icon group, the version information and the manifest.
func object(images [][]byte, manifest, version []byte) []byte {
	const (
		typeIcon      = 3
		typeGroupIcon = 14
		typeVersion   = 16
		typeManifest  = 24
	)
	group := []byte{0, 0, 1, 0, byte(len(images)), 0}
	resources := map[int]map[int][]byte{typeIcon: {}, typeGroupIcon: {}, typeVersion: {1: version}, typeManifest: {1: manifest}}
	for i, data := range images {
		resources[typeIcon][i+1] = data
		group = append(group, entry(sizes[i], len(data))...)
		group = binary.LittleEndian.AppendUint16(group, uint16(i+1))
	}
	resources[typeGroupIcon][1] = group

	le := binary.LittleEndian
	var section []byte
	reserve := func(n int) int {
		at := len(section)
		section = append(section, make([]byte, n)...)
		return at
	}
	// directory reserves a table with one row per key and returns its offset.
	directory := func(count int) int {
		at := reserve(16 + 8*count)
		le.PutUint16(section[at+14:], uint16(count))
		return at
	}
	keys := func(m map[int][]byte) []int {
		out := make([]int, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Ints(out)
		return out
	}
	types := make([]int, 0, len(resources))
	for kind := range resources {
		types = append(types, kind)
	}
	sort.Ints(types)

	type leaf struct {
		at   int
		data []byte
	}
	var leaves []leaf
	root := directory(len(types))
	for ti, kind := range types {
		names := keys(resources[kind])
		typed := directory(len(names))
		le.PutUint32(section[root+16+8*ti:], uint32(kind))
		le.PutUint32(section[root+20+8*ti:], uint32(typed)|0x80000000)
		for ni, name := range names {
			language := directory(1)
			le.PutUint32(section[typed+16+8*ni:], uint32(name))
			le.PutUint32(section[typed+20+8*ni:], uint32(language)|0x80000000)
			at := reserve(16)
			le.PutUint32(section[language+20:], uint32(at))
			leaves = append(leaves, leaf{at, resources[kind][name]})
		}
	}
	var relocations []byte
	for _, l := range leaves {
		for len(section)%4 != 0 {
			section = append(section, 0)
		}
		le.PutUint32(section[l.at:], uint32(len(section)))
		le.PutUint32(section[l.at+4:], uint32(len(l.data)))
		section = append(section, l.data...)
		// The linker adds the section's address to each data offset.
		relocations = le.AppendUint32(relocations, uint32(l.at))
		relocations = le.AppendUint32(relocations, 0)
		relocations = le.AppendUint16(relocations, 3)
	}

	const headers = 20 + 40
	var out bytes.Buffer
	binary.Write(&out, le, struct {
		Machine, Sections          uint16
		Time, Symbols, SymbolCount uint32
		OptionalSize, Flags        uint16
	}{Machine: 0x8664, Sections: 1, Symbols: uint32(headers + len(section) + len(relocations)), SymbolCount: 1})
	binary.Write(&out, le, struct {
		Name                           [8]byte
		VirtualSize, VirtualAddress    uint32
		Size, Data, Relocations, Lines uint32
		RelocationCount, LineCount     uint16
		Flags                          uint32
	}{Name: [8]byte{'.', 'r', 's', 'r', 'c'}, Size: uint32(len(section)), Data: headers,
		Relocations: uint32(headers + len(section)), RelocationCount: uint16(len(leaves)), Flags: 0x40000040})
	out.Write(section)
	out.Write(relocations)
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 3, 0}) // the section's symbol
	out.Write([]byte{4, 0, 0, 0})                                                     // an empty string table
	return out.Bytes()
}
