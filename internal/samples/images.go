// Package samples builds small files that carry known metadata. The tests,
// the release check and the documentation all use them, so no real person's
// files are needed.
package samples

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
)

// Values the sample files carry. The position is the Royal Observatory, Greenwich.
const (
	Author    = "Alex Example"
	Camera    = "Northlight"
	Model     = "NL-200"
	Serial    = "NL200-0048213"
	Software  = "Northlight Firmware 2.1"
	Taken     = "2026:03:14 09:26:53"
	Latitude  = 51.476852
	Longitude = -0.000500
	Comment   = "Edited on the office laptop"
	Hidden    = "PK\x03\x04 notes that were never meant to be shared"
)

// XMP is the packet embedded in the sample pictures.
const XMP = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:xmpMM="http://ns.adobe.com/xap/1.0/mm/"
    xmlns:stEvt="http://ns.adobe.com/xap/1.0/sType/ResourceEvent#"
    xmlns:photoshop="http://ns.adobe.com/photoshop/1.0/"
    xmp:CreatorTool="Northlight Studio 4.2 (Windows)"
    xmp:CreateDate="2026-03-14T09:26:53Z"
    xmpMM:DocumentID="xmp.did:6f0c2a4e-1d2b-4c55-9a10-3f5a7e2b9c41"
    photoshop:City="Greenwich">
   <dc:creator><rdf:Seq><rdf:li>Alex Example</rdf:li></rdf:Seq></dc:creator>
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">Morning at the observatory</rdf:li></rdf:Alt></dc:title>
   <dc:subject><rdf:Bag><rdf:li>observatory</rdf:li><rdf:li>london</rdf:li></rdf:Bag></dc:subject>
   <xmpMM:History><rdf:Seq>
    <rdf:li rdf:parseType="Resource"><stEvt:action>created</stEvt:action><stEvt:when>2026-03-14T09:26:53Z</stEvt:when><stEvt:softwareAgent>Northlight Studio 4.2 (Windows)</stEvt:softwareAgent></rdf:li>
    <rdf:li rdf:parseType="Resource"><stEvt:action>saved</stEvt:action><stEvt:when>2026-03-15T18:02:11Z</stEvt:when><stEvt:softwareAgent>Northlight Studio 4.2 (Windows)</stEvt:softwareAgent></rdf:li>
   </rdf:Seq></xmpMM:History>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

// picture draws a simple scene so the samples are real, decodable images.
func picture(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.RGBA{uint8(40 + 120*y/height), uint8(90 + 100*y/height), uint8(200 - 60*y/height), 255}
			if y > height*2/3 {
				c = color.RGBA{uint8(30 + 40*x/width), uint8(70 + 30*x/width), 50, 255}
			}
			dx, dy := x-width*3/4, y-height/4
			if dx*dx+dy*dy < (height/8)*(height/8) {
				c = color.RGBA{250, 230, 150, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func rational(n, d uint32) []byte {
	return binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil, n), d)
}

func degrees(v float64) []byte {
	if v < 0 {
		v = -v
	}
	d := uint32(v)
	m := uint32((v - float64(d)) * 60)
	s := uint32(((v-float64(d))*60 - float64(m)) * 60 * 10000)
	return append(append(rational(d, 1), rational(m, 1)...), rational(s, 10000)...)
}

type field struct {
	tag, kind uint16
	count     uint32
	value     []byte
}

func ascii(tag uint16, s string) field {
	return field{tag, 2, uint32(len(s) + 1), append([]byte(s), 0)}
}

func short(tag uint16, v uint16) field {
	return field{tag, 3, 1, binary.LittleEndian.AppendUint16(nil, v)}
}

func long(tag uint16, v uint32) field {
	return field{tag, 4, 1, binary.LittleEndian.AppendUint32(nil, v)}
}

// directory lays out one TIFF directory at the given offset and returns its
// bytes. Values longer than four bytes follow the entry table.
func directory(offset uint32, fields []field, next uint32) []byte {
	le := binary.LittleEndian
	table := 2 + 12*len(fields) + 4
	out := make([]byte, table)
	le.PutUint16(out, uint16(len(fields)))
	for i, f := range fields {
		e := out[2+12*i:]
		le.PutUint16(e, f.tag)
		le.PutUint16(e[2:], f.kind)
		le.PutUint32(e[4:], f.count)
		if len(f.value) <= 4 {
			copy(e[8:], f.value)
			continue
		}
		le.PutUint32(e[8:], offset+uint32(len(out)))
		out = append(out, f.value...)
		if len(out)%2 == 1 {
			out = append(out, 0)
		}
	}
	le.PutUint32(out[2+12*len(fields):], next)
	return out
}

// EXIF builds a TIFF block with camera, photo, GPS and thumbnail data.
func EXIF(orientation uint16) []byte {
	var thumb bytes.Buffer
	jpeg.Encode(&thumb, picture(32, 24), &jpeg.Options{Quality: 60})

	// Sizes are needed before offsets, so each directory is built twice.
	build := func(at [4]uint32) [4][]byte {
		main := directory(8, []field{
			ascii(0x010F, Camera), ascii(0x0110, Model), short(0x0112, orientation),
			ascii(0x0131, Software), ascii(0x0132, Taken), ascii(0x013B, Author),
			long(0x8769, at[1]), long(0x8825, at[2]),
		}, at[3])
		photo := directory(at[1], []field{
			{0x829A, 5, 1, rational(1, 250)}, {0x829D, 5, 1, rational(18, 10)},
			short(0x8827, 200), ascii(0x9003, Taken), ascii(0x9011, "+00:00"),
			{0x920A, 5, 1, rational(26, 1)}, ascii(0xA431, Serial), ascii(0xA434, "Northlight 26mm f/1.8"),
		}, 0)
		north, east := "N", "E"
		if Longitude < 0 {
			east = "W"
		}
		gps := directory(at[2], []field{
			{0x0000, 1, 4, []byte{2, 3, 0, 0}}, ascii(0x0001, north), {0x0002, 5, 3, degrees(Latitude)},
			ascii(0x0003, east), {0x0004, 5, 3, degrees(Longitude)}, {0x0005, 1, 1, []byte{0}},
			{0x0006, 5, 1, rational(470, 10)}, ascii(0x001D, "2026:03:14"),
		}, 0)
		small := directory(at[3], []field{
			short(0x0103, 6), long(0x0201, at[3]+2+12*3+4), long(0x0202, uint32(thumb.Len())),
		}, 0)
		return [4][]byte{main, photo, gps, small}
	}
	parts := build([4]uint32{})
	var at [4]uint32
	at[1] = 8 + uint32(len(parts[0]))
	at[2] = at[1] + uint32(len(parts[1]))
	at[3] = at[2] + uint32(len(parts[2]))
	parts = build(at)

	out := []byte("II*\x00\x08\x00\x00\x00")
	for _, p := range parts {
		out = append(out, p...)
	}
	return append(out, thumb.Bytes()...)
}

// IPTC builds a Photoshop resource block with press caption data.
func IPTC() []byte {
	var records []byte
	for _, r := range []struct {
		number byte
		text   string
	}{{80, Author}, {90, "Greenwich"}, {101, "United Kingdom"}, {116, "(c) 2026 Alex Example"}, {120, "Sunrise over the observatory"}} {
		records = append(records, 0x1C, 2, r.number, byte(len(r.text)>>8), byte(len(r.text)))
		records = append(records, r.text...)
	}
	out := []byte("8BIM\x04\x04\x00\x00")
	out = binary.BigEndian.AppendUint32(out, uint32(len(records)))
	out = append(out, records...)
	if len(out)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func segment(marker byte, body []byte) []byte {
	return append([]byte{0xFF, marker, byte((len(body) + 2) >> 8), byte(len(body) + 2)}, body...)
}

// JPEG builds a photo with EXIF, XMP, IPTC, a comment and data hidden after the picture.
func JPEG(orientation uint16) []byte {
	var plain bytes.Buffer
	jpeg.Encode(&plain, picture(320, 240), &jpeg.Options{Quality: 85})
	out := []byte{0xFF, 0xD8}
	out = append(out, segment(0xE0, []byte("JFIF\x00\x01\x02\x01\x00\x48\x00\x48\x00\x00"))...)
	out = append(out, segment(0xE1, append([]byte("Exif\x00\x00"), EXIF(orientation)...))...)
	out = append(out, segment(0xE1, append([]byte("http://ns.adobe.com/xap/1.0/\x00"), XMP...))...)
	out = append(out, segment(0xED, append([]byte("Photoshop 3.0\x00"), IPTC()...))...)
	out = append(out, segment(0xFE, []byte(Comment))...)
	out = append(out, plain.Bytes()[2:]...)
	return append(out, Hidden...)
}

func chunk(kind string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, kind...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[4:]))
}

// PNG builds a picture with text chunks, XMP, EXIF, a timestamp and hidden data.
func PNG() []byte {
	var plain bytes.Buffer
	png.Encode(&plain, picture(320, 240))
	b := plain.Bytes()
	head := 8 + 25 // signature and the header chunk
	var packed bytes.Buffer
	w := zlib.NewWriter(&packed)
	w.Write([]byte("Exported for the client review"))
	w.Close()

	out := append([]byte(nil), b[:head]...)
	out = append(out, chunk("pHYs", []byte{0, 0, 0x0E, 0xC3, 0, 0, 0x0E, 0xC3, 1})...)
	out = append(out, chunk("tEXt", []byte("Author\x00"+Author))...)
	out = append(out, chunk("tEXt", []byte("Software\x00"+Software))...)
	out = append(out, chunk("zTXt", append([]byte("Comment\x00\x00"), packed.Bytes()...))...)
	out = append(out, chunk("iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00"+XMP))...)
	out = append(out, chunk("tEXt", []byte("parameters\x00a lighthouse at dawn, 35mm photo\nSteps: 30, Seed: 1234567890"))...)
	out = append(out, chunk("eXIf", EXIF(1))...)
	out = append(out, chunk("tIME", []byte{0x07, 0xEA, 3, 14, 9, 26, 53})...)
	out = append(out, chunk("prVW", bytes.Repeat([]byte{0x55}, 64))...)
	out = append(out, b[head:]...)
	return append(out, Hidden...)
}

// GIF builds a two-frame animation with a comment, XMP and hidden data.
func GIF() []byte {
	palette := color.Palette{color.RGBA{20, 40, 90, 255}, color.RGBA{250, 230, 150, 255}}
	anim := &gif.GIF{LoopCount: 0}
	for frame := 0; frame < 2; frame++ {
		img := image.NewPaletted(image.Rect(0, 0, 64, 48), palette)
		for y := 16; y < 32; y++ {
			for x := 8 + 24*frame; x < 24+24*frame; x++ {
				img.SetColorIndex(x, y, 1)
			}
		}
		anim.Image = append(anim.Image, img)
		anim.Delay = append(anim.Delay, 40)
	}
	var plain bytes.Buffer
	gif.EncodeAll(&plain, anim)
	b := plain.Bytes()
	body := len(b) - 1 // everything before the trailer byte

	comment := append([]byte{0x21, 0xFE, byte(len(Comment))}, Comment...)
	comment = append(comment, 0)
	packet := append([]byte{0x21, 0xFF, 11}, "XMP DataXMP"...)
	packet = append(packet, XMP...)
	packet = append(packet, 0x01)
	for i := 0xFF; i >= 0; i-- {
		packet = append(packet, byte(i))
	}
	packet = append(packet, 0)

	out := append([]byte(nil), b[:body]...)
	out = append(out, comment...)
	out = append(out, packet...)
	out = append(out, 0x3B)
	return append(out, Hidden...)
}

// WebP builds a minimal lossless picture in the extended layout with EXIF and XMP chunks.
func WebP(orientation uint16) []byte {
	riff := func(kind string, data []byte) []byte {
		out := append([]byte(kind), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
		out = append(out, data...)
		if len(data)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	body := []byte("WEBP")
	body = append(body, riff("VP8X", []byte{0x0C, 0, 0, 0, 0, 0, 0, 0, 0, 0})...)
	body = append(body, riff("VP8L", []byte{0x2F, 0, 0, 0, 0x10, 0x07, 0x10, 0x11, 0x11, 0x88, 0x88, 0xFE, 0x07, 0})...)
	body = append(body, riff("EXIF", EXIF(orientation))...)
	body = append(body, riff("XMP ", []byte(XMP))...)
	body = append(body, riff("NOTE", []byte("internal build 4471"))...)
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	out = append(out, body...)
	return append(out, Hidden...)
}

// Secrets lists text that the sample files carry as metadata. None of it may
// survive cleaning.
func Secrets() []string {
	return []string{
		Author, Camera, Model, Serial, Software, Taken, Comment, "Greenwich", "United Kingdom",
		"Northlight Studio", "observatory", "xmp.did:", "lighthouse", "client review", "never meant to be shared",
		"internal build",
	}
}
