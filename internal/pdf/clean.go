package pdf

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"sort"
	"strconv"

	"lazascrub/internal/jpeg"
)

// ErrEncrypted is returned for password-protected documents, whose text
// properties cannot be read or rewritten without the key.
var ErrEncrypted = errors.New("the PDF is password protected, so its metadata cannot be removed. Remove the password first")

// cut replaces a span of an object's source bytes.
type cut struct {
	start, end int
	with       []byte
}

// plan describes how one object is written to the cleaned file.
type plan struct {
	obj    *object
	cuts   []cut
	stream []byte
}

// reach collects every object the catalog leads to, skipping the keys that
// cleaning removes. Whatever it does not reach is left out of the new file:
// document information, XMP packets, thumbnails and everything that earlier
// saves left behind.
func (d *document) reach(root int) (plans map[int]*plan, missing []int) {
	plans = map[int]*plan{}
	queue := []int{root}
	var walk func(v Object, top bool, pl *plan)
	walk = func(v Object, top bool, pl *plan) {
		switch v := v.(type) {
		case Ref:
			queue = append(queue, v.Num)
		case Array:
			for _, item := range v {
				walk(item, false, pl)
			}
		case *Dict:
			isMarkup := markup(v)
			for _, e := range v.entries {
				switch {
				case removable(v, e.key, isMarkup):
					pl.cuts = append(pl.cuts, cut{e.start, e.end, nil})
				case top && e.key == "Length":
				default:
					walk(e.value, false, pl)
				}
			}
		}
	}
	for len(queue) > 0 {
		num := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if _, done := plans[num]; done {
			continue
		}
		o, err := d.get(num)
		if err != nil {
			// A reference to a missing object is legal and reads as null.
			plans[num] = nil
			continue
		}
		pl := &plan{obj: o, stream: o.stream}
		plans[num] = pl
		walk(o.value, o.isStream, pl)
	}
	for num, pl := range plans {
		if pl == nil || num <= 0 {
			delete(plans, num)
			missing = append(missing, num)
		}
	}
	return plans, missing
}

// body returns the object's value with the planned cuts applied.
func (pl *plan) body() []byte {
	o := pl.obj
	cuts := append([]cut(nil), pl.cuts...)
	if o.isStream {
		dict := o.value.(*Dict)
		length := []byte("/Length " + strconv.Itoa(len(pl.stream)))
		placed := false
		for _, e := range dict.entries {
			if e.key == "Length" && !placed {
				cuts = append(cuts, cut{e.start, e.end, length})
				placed = true
			} else if e.key == "Length" {
				cuts = append(cuts, cut{e.start, e.end, nil})
			}
		}
		if !placed {
			cuts = append(cuts, cut{dict.end - 2, dict.end - 2, append(length, ' ')})
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].start < cuts[j].start })
	var out bytes.Buffer
	pos := o.start
	for _, c := range cuts {
		if c.start < pos {
			continue
		}
		out.Write(o.src[pos:c.start])
		out.Write(c.with)
		pos = c.end
	}
	out.Write(o.src[pos:o.end])
	return out.Bytes()
}

// Clean rebuilds the document from the objects it still uses. Document
// information, XMP, thumbnails, private editing data, reviewer names and
// every earlier revision are left out. Pictures stored as JPEG are cleaned too.
func Clean(data []byte) ([]byte, error) {
	d, err := load(data)
	if err != nil {
		return nil, err
	}
	if d.trailer("Encrypt") != nil {
		return nil, ErrEncrypted
	}
	root, ok := d.trailer("Root").(Ref)
	if !ok {
		return nil, errors.New("the PDF has no document catalog, so it cannot be cleaned")
	}
	pages := d.pages()
	if pages == 0 {
		return nil, errors.New("no pages could be found in the PDF, so it was not cleaned")
	}
	plans, missing := d.reach(root.Num)
	for _, num := range missing {
		// A reference to an object that was deleted is harmless. A reference
		// to one that is in the file but not in its table means the table
		// cannot be trusted, and neither could a copy built from it.
		if d.present[num] && !d.freed[num] {
			return nil, errors.New("the PDF could not be read reliably, so it was not cleaned")
		}
	}
	for _, pl := range plans {
		if !pl.obj.isStream || soleFilter(pl.obj.value.(*Dict)) != "DCTDecode" {
			continue
		}
		if cleaned, err := jpeg.Clean(pl.obj.stream); err == nil && !bytes.Equal(cleaned, pl.obj.stream) {
			pl.stream = cleaned
		}
	}
	out := write(d, plans, root)

	// Read the result back before trusting it: it must open with its own
	// table, hold the same pages and lead to the same number of objects.
	check, err := load(out)
	if err != nil || check.rebuilt || check.pages() != pages {
		return nil, errors.New("the cleaned PDF did not read back correctly, so it was not saved")
	}
	if copied, lost := check.reach(root.Num); len(copied) != len(plans) || len(lost) != len(missing) {
		return nil, errors.New("the cleaned PDF did not read back correctly, so it was not saved")
	}
	return out, nil
}

func write(d *document, plans map[int]*plan, root Ref) []byte {
	nums := make([]int, 0, len(plans))
	for num := range plans {
		nums = append(nums, num)
	}
	sort.Ints(nums)

	var out bytes.Buffer
	version := "%PDF-1.7"
	if head := d.data[d.base:]; len(head) >= 8 && head[5] >= '1' && head[5] <= '9' && head[6] == '.' && head[7] >= '0' && head[7] <= '9' {
		version = string(head[:8])
	}
	out.WriteString(version + "\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(nums))
	for i, num := range nums {
		pl := plans[num]
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d %d obj\n", num, pl.obj.gen)
		out.Write(pl.body())
		if pl.obj.isStream {
			out.WriteString("\nstream\n")
			out.Write(pl.stream)
			out.WriteString("\nendstream")
		}
		out.WriteString("\nendobj\n")
	}
	table := out.Len()
	out.WriteString("xref\n0 1\n0000000000 65535 f \n")
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		fmt.Fprintf(&out, "%d %d\n", nums[i], j-i+1)
		for k := i; k <= j; k++ {
			fmt.Fprintf(&out, "%010d %05d n \n", offsets[k], min(plans[nums[k]].obj.gen, 65535))
		}
		i = j + 1
	}
	size := 1
	if len(nums) > 0 {
		size = nums[len(nums)-1] + 1
	}
	// The file identifier is left out on purpose. It is optional, and the
	// original one would link the cleaned copy to the file it came from.
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d %d R >>\nstartxref\n%d\n%%%%EOF\n", size, root.Num, root.Gen, table)
	return out.Bytes()
}

// Fingerprint hashes everything the document displays: every object the
// catalog leads to, without the metadata that Clean removes.
func Fingerprint(data []byte) ([]byte, error) {
	d, err := load(data)
	if err != nil {
		return nil, err
	}
	root, ok := d.trailer("Root").(Ref)
	if !ok {
		return nil, errors.New("the PDF has no document catalog")
	}
	plans, _ := d.reach(root.Num)
	nums := make([]int, 0, len(plans))
	for num := range plans {
		nums = append(nums, num)
	}
	sort.Ints(nums)
	h := sha256.New()
	for _, num := range nums {
		o := plans[num].obj
		fmt.Fprintf(h, "\n%d ", num)
		canonical(h, o.value, o.isStream)
		if !o.isStream {
			continue
		}
		var content []byte
		if soleFilter(o.value.(*Dict)) == "DCTDecode" {
			content, _ = jpeg.Fingerprint(o.stream)
		}
		if content == nil {
			sum := sha256.Sum256(o.stream)
			content = sum[:]
		}
		h.Write(content)
	}
	return h.Sum(nil), nil
}

// canonical writes a value in one fixed form, leaving out what Clean removes.
func canonical(h hash.Hash, v Object, top bool) {
	switch v := v.(type) {
	case nil:
		h.Write([]byte("null"))
	case bool, int64, float64:
		fmt.Fprint(h, v)
	case String:
		fmt.Fprintf(h, "<%x>", []byte(v))
	case Name:
		fmt.Fprintf(h, "/%s", string(v))
	case Ref:
		fmt.Fprintf(h, "%d %d R", v.Num, v.Gen)
	case Array:
		h.Write([]byte("["))
		for _, item := range v {
			canonical(h, item, false)
			h.Write([]byte(" "))
		}
		h.Write([]byte("]"))
	case *Dict:
		isMarkup := markup(v)
		h.Write([]byte("<<"))
		for _, e := range v.entries {
			if removable(v, e.key, isMarkup) || (top && e.key == "Length") {
				continue
			}
			fmt.Fprintf(h, "/%s ", string(e.key))
			canonical(h, e.value, false)
		}
		h.Write([]byte(">>"))
	}
}

// Pages returns the number of pages in a PDF.
func Pages(data []byte) (int, error) {
	d, err := load(data)
	if err != nil {
		return 0, err
	}
	return d.pages(), nil
}
