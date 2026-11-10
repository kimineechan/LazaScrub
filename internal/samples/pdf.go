package samples

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
)

// Text that the sample PDF held before its second save. A cleaned copy must not contain it.
const (
	DraftTitle = "Draft budget (confidential)"
	DraftText  = "Confidential draft: do not circulate"
	FinalTitle = "Budget summary"
	FinalText  = "Budget summary for the board"
	Reviewer   = "Sam Reviewer"
)

// document writes PDF objects and remembers where each one starts.
type document struct {
	bytes.Buffer
	offsets map[int]int
}

func (d *document) object(num int, body string) {
	d.offsets[num] = d.Len()
	fmt.Fprintf(d, "%d 0 obj\n%s\nendobj\n", num, body)
}

func (d *document) stream(num int, dict string, data []byte) {
	d.offsets[num] = d.Len()
	fmt.Fprintf(d, "%d 0 obj\n<< %s /Length %d >>\nstream\n", num, dict, len(data))
	d.Write(data)
	d.WriteString("\nendstream\nendobj\n")
}

// table writes a cross-reference table for the given objects and returns its offset.
func (d *document) table(nums []int, trailer string) int {
	sort.Ints(nums)
	at := d.Len()
	d.WriteString("xref\n0 1\n0000000000 65535 f \n")
	for _, num := range nums {
		fmt.Fprintf(d, "%d 1\n%010d 00000 n \n", num, d.offsets[num])
	}
	fmt.Fprintf(d, "trailer\n<< %s >>\nstartxref\n%d\n%%%%EOF\n", trailer, at)
	return at
}

func info(title string) string {
	return fmt.Sprintf("<< /Title (%s) /Author (%s) /Creator (Northlight Writer 12) /Producer (Northlight PDF Library 3.4) "+
		"/CreationDate (D:20260314092653+01'00') /ModDate (D:20260315180211Z) /Company (Example Holdings Ltd) >>", title, Author)
}

func page(text string) []byte {
	return []byte(fmt.Sprintf("BT /F1 18 Tf 72 720 Td (%s) Tj ET\nq 200 0 0 150 72 520 cm /Im1 Do Q", text))
}

// PDF builds a one-page document that was saved twice. The first save held a
// confidential draft; the second replaced its title and text but, as PDF
// editors do, left the old objects in the file. It also carries XMP, a
// reviewer's comment, a page thumbnail and a photo with GPS data.
func PDF() []byte {
	d := &document{offsets: map[int]int{}}
	d.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	d.object(1, "<< /Type /Catalog /Pages 2 0 R /Metadata 6 0 R >>")
	d.object(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	d.object(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Thumb 7 0 R "+
		"/Resources << /Font << /F1 5 0 R >> /XObject << /Im1 9 0 R >> >> /Annots [8 0 R] "+
		"/PieceInfo << /Northlight << /LastModified (D:20260315180211Z) /Private << /Zoom 125 >> >> >> >>")
	d.stream(4, "", page(DraftText))
	d.object(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	d.stream(6, "/Type /Metadata /Subtype /XML", []byte(XMP))
	d.stream(7, "/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8",
		[]byte{0x20, 0x60, 0xA0, 0xE0})
	d.object(8, fmt.Sprintf("<< /Type /Annot /Subtype /Text /Rect [400 700 420 720] /T (%s) "+
		"/M (D:20260315175900Z) /Contents (Check this figure before it goes out) >>", Reviewer))
	d.stream(9, "/Type /XObject /Subtype /Image /Width 320 /Height 240 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode",
		JPEG(1))
	d.object(10, info(DraftTitle))
	first := d.table([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		"/Size 11 /Root 1 0 R /Info 10 0 R /ID [<8F1C2B7A90E44D3BA1C5D6E7F8091A2B> <8F1C2B7A90E44D3BA1C5D6E7F8091A2B>]")

	// The second save appends new versions of the text and the properties.
	d.stream(4, "", page(FinalText))
	d.object(10, info(FinalTitle))
	d.table([]int{4, 10}, fmt.Sprintf(
		"/Size 11 /Root 1 0 R /Info 10 0 R /Prev %d /ID [<8F1C2B7A90E44D3BA1C5D6E7F8091A2B> <3D4E5F60718293A4B5C6D7E8F9A0B1C2>]", first))
	return d.Bytes()
}

func deflate(data []byte) []byte {
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	w.Write(data)
	w.Close()
	return out.Bytes()
}

// CompressedPDF builds a document in the layout modern apps write: most
// objects packed into a compressed object stream, located through a
// cross-reference stream with a row predictor.
func CompressedPDF() []byte {
	d := &document{offsets: map[int]int{}}
	d.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	d.stream(4, "", page(FinalText))
	d.stream(6, "/Type /Metadata /Subtype /XML", []byte(XMP))
	d.stream(9, "/Type /XObject /Subtype /Image /Width 320 /Height 240 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode",
		JPEG(1))

	members := []struct {
		num  int
		body string
	}{
		{1, "<< /Type /Catalog /Pages 2 0 R /Metadata 6 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> /XObject << /Im1 9 0 R >> >> >>"},
		{5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"},
		{10, info(FinalTitle)},
	}
	var index, bodies bytes.Buffer
	for _, m := range members {
		fmt.Fprintf(&index, "%d %d ", m.num, bodies.Len())
		bodies.WriteString(m.body + "\n")
	}
	packed := deflate(append(index.Bytes(), bodies.Bytes()...))
	d.stream(11, fmt.Sprintf("/Type /ObjStm /N %d /First %d /Filter /FlateDecode", len(members), index.Len()), packed)

	// One row per object: type, three bytes of offset or stream number, one byte of index.
	rows := make([][5]byte, 13)
	for num, at := range d.offsets {
		rows[num] = [5]byte{1, byte(at >> 16), byte(at >> 8), byte(at), 0}
	}
	for i, m := range members {
		rows[m.num] = [5]byte{2, 0, 0, 11, byte(i)}
	}
	at := d.Len()
	rows[12] = [5]byte{1, byte(at >> 16), byte(at >> 8), byte(at), 0}
	var predicted []byte
	var prev [5]byte
	for _, row := range rows {
		predicted = append(predicted, 2)
		for i := range row {
			predicted = append(predicted, row[i]-prev[i])
		}
		prev = row
	}
	d.stream(12, "/Type /XRef /Size 13 /W [1 3 1] /Root 1 0 R /Info 10 0 R /Filter /FlateDecode "+
		"/DecodeParms << /Predictor 12 /Columns 5 >>", deflate(predicted))
	fmt.Fprintf(d, "startxref\n%d\n%%%%EOF\n", at)
	return d.Bytes()
}
