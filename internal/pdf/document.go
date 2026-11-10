package pdf

import (
	"bytes"
	"errors"
	"sort"
)

// object is one indirect object, parsed.
type object struct {
	num, gen   int
	value      Object
	src        []byte // the bytes the value was parsed from
	start, end int    // where the value sits in src
	isStream   bool
	stream     []byte // the stream data as stored, still encoded
	next       int    // file offset just past the object
}

// place says where the current version of an object lives.
type place struct {
	offset int // position in the file, or -1 when it is inside an object stream
	holder int // number of the object stream that contains it
	index  int // position inside that object stream
	free   bool
}

// spot is an object header found by reading the file from start to end,
// whether or not the document still uses that object.
type spot struct {
	num, offset int
	obj         *object
}

// holder is a decoded object stream.
type holder struct {
	nums   []int
	starts []int
	data   []byte
}

type document struct {
	data      []byte
	base      int // offset of the %PDF header
	table     map[int]place
	trailers  []*Dict // newest first
	revisions int
	linear    bool
	spots     []spot
	latest    map[int]*object // the last copy of each object in file order
	at        map[int]*object // objects by file offset
	objects   map[int]*object // objects by number
	holders   map[int]*holder
	nesting   int
}

var (
	errNotPDF  = errors.New("not a PDF file")
	errMissing = errors.New("the PDF refers to an object that is not in the file")
)

// Is reports whether data looks like a PDF file.
func Is(data []byte) bool {
	return bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-"))
}

func load(data []byte) (*document, error) {
	base := bytes.Index(data[:min(len(data), 1024)], []byte("%PDF-"))
	if base < 0 {
		return nil, errNotPDF
	}
	d := &document{
		data: data, base: base, table: map[int]place{},
		at: map[int]*object{}, objects: map[int]*object{}, holders: map[int]*holder{}, latest: map[int]*object{},
	}
	tableErr := d.readTable()
	d.survey()
	if len(d.spots) > 0 {
		first, _ := d.spots[0].obj.value.(*Dict)
		d.linear = first.has("Linearized")
	}
	if tableErr != nil || d.catalog() == nil {
		d.rebuild()
	}
	if d.catalog() == nil {
		return nil, errors.New("the PDF has no document catalog, so it cannot be read")
	}
	// A file prepared for fast web view has two cross-reference sections for one save.
	if d.linear && d.revisions > 1 {
		d.revisions--
	}
	return d, nil
}

// trailer looks a key up in the trailers, newest first.
func (d *document) trailer(key Name) Object {
	for _, t := range d.trailers {
		if v := t.Get(key); v != nil {
			return v
		}
	}
	return nil
}

func (d *document) catalog() *Dict {
	return d.dict(d.trailer("Root"))
}

// readTable follows the chain of cross-reference sections from the end of the file.
func (d *document) readTable() error {
	tail := max(0, len(d.data)-2048)
	i := bytes.LastIndex(d.data[tail:], []byte("startxref"))
	if i < 0 {
		return errSyntax
	}
	p := &parser{src: d.data, pos: tail + i + len("startxref")}
	offset, ok := p.integer()
	if !ok {
		return errSyntax
	}
	seen := map[int]bool{}
	for offset >= 0 && !seen[offset] {
		seen[offset] = true
		trailer, prev, err := d.section(offset)
		if err != nil && d.base > 0 {
			trailer, prev, err = d.section(offset + d.base)
		}
		if err != nil {
			return err
		}
		d.trailers = append(d.trailers, trailer)
		d.revisions++
		offset = prev
	}
	for num, pl := range d.table {
		if pl.free {
			delete(d.table, num)
		}
	}
	return nil
}

func (d *document) set(num int, pl place) {
	if _, taken := d.table[num]; !taken {
		d.table[num] = pl
	}
}

// section reads one cross-reference table or stream and returns its trailer
// and the offset of the section before it.
func (d *document) section(offset int) (*Dict, int, error) {
	if offset >= len(d.data) {
		return nil, 0, errSyntax
	}
	p := &parser{src: d.data, pos: offset}
	if !p.keyword("xref") {
		o, err := d.parseAt(offset)
		if err != nil {
			return nil, 0, err
		}
		dict, _ := o.value.(*Dict)
		if !o.isStream || dict.Get("Type") != Name("XRef") {
			return nil, 0, errSyntax
		}
		if err := d.crossStream(dict, o.stream); err != nil {
			return nil, 0, err
		}
		return dict, previous(dict), nil
	}
	type pending struct {
		num int
		pl  place
	}
	var rows []pending
	for !p.keyword("trailer") {
		first, ok1 := p.integer()
		count, ok2 := p.integer()
		if !ok1 || !ok2 || count < 0 || count > len(d.data)/10 {
			return nil, 0, errSyntax
		}
		for i := 0; i < count; i++ {
			at, ok1 := p.integer()
			_, ok2 := p.integer()
			p.space()
			kind := p.word()
			if !ok1 || !ok2 || (kind != "n" && kind != "f") {
				return nil, 0, errSyntax
			}
			rows = append(rows, pending{first + i, place{offset: at, free: kind == "f"}})
		}
	}
	v, err := p.object()
	trailer, ok := v.(*Dict)
	if err != nil || !ok {
		return nil, 0, errSyntax
	}
	// A hybrid file lists its compressed objects in a separate stream, which
	// takes priority over the table.
	if at, ok := asInt(trailer.Get("XRefStm")); ok {
		if o, err := d.parseAt(at); err == nil && o.isStream {
			d.crossStream(o.value.(*Dict), o.stream)
		}
	}
	for _, r := range rows {
		d.set(r.num, r.pl)
	}
	return trailer, previous(trailer), nil
}

func previous(trailer *Dict) int {
	if prev, ok := asInt(trailer.Get("Prev")); ok {
		return prev
	}
	return -1
}

func (d *document) crossStream(dict *Dict, raw []byte) error {
	data, err := decode(dict, raw)
	if err != nil {
		return err
	}
	widths := asInts(dict.Get("W"))
	size, _ := asInt(dict.Get("Size"))
	ranges := asInts(dict.Get("Index"))
	if ranges == nil {
		ranges = []int{0, size}
	}
	if len(widths) != 3 || widths[0] < 0 || widths[1] < 0 || widths[2] < 0 {
		return errSyntax
	}
	row := widths[0] + widths[1] + widths[2]
	if row == 0 || row > 24 {
		return errSyntax
	}
	pos := 0
	for k := 0; k+1 < len(ranges); k += 2 {
		for i := 0; i < ranges[k+1] && pos+row <= len(data); i++ {
			var f [3]int
			at := pos
			for n, w := range widths {
				for j := 0; j < w; j++ {
					f[n] = f[n]<<8 | int(data[at])
					at++
				}
			}
			if widths[0] == 0 {
				f[0] = 1
			}
			pos += row
			switch f[0] {
			case 0:
				d.set(ranges[k]+i, place{free: true})
			case 1:
				d.set(ranges[k]+i, place{offset: f[1]})
			case 2:
				d.set(ranges[k]+i, place{offset: -1, holder: f[1], index: f[2]})
			}
		}
	}
	return nil
}

// parseAt reads the indirect object that starts at a file offset.
func (d *document) parseAt(offset int) (*object, error) {
	if o, ok := d.at[offset]; ok {
		return o, nil
	}
	if offset < 0 || offset >= len(d.data) {
		return nil, errMissing
	}
	p := &parser{src: d.data, pos: offset}
	num, ok1 := p.integer()
	gen, ok2 := p.integer()
	if !ok1 || !ok2 || !p.keyword("obj") {
		return nil, errSyntax
	}
	p.space()
	o := &object{num: num, gen: gen, src: d.data, start: p.pos}
	value, err := p.object()
	if err != nil {
		return nil, err
	}
	o.value, o.end = value, p.pos
	p.space()
	if dict, ok := value.(*Dict); ok && bytes.HasPrefix(d.data[p.pos:], []byte("stream")) {
		at := p.pos + len("stream")
		if at < len(d.data) && d.data[at] == '\r' {
			at++
		}
		if at < len(d.data) && d.data[at] == '\n' {
			at++
		}
		end := -1
		if length, ok := d.length(dict.Get("Length")); ok && length >= 0 && at+length <= len(d.data) {
			q := &parser{src: d.data, pos: at + length}
			if q.keyword("endstream") {
				end, p.pos = at+length, q.pos
			}
		}
		if end < 0 {
			// The stated length is missing or wrong, so find the end marker.
			i := bytes.Index(d.data[at:], []byte("endstream"))
			if i < 0 {
				return nil, errSyntax
			}
			end = at + i
			if end > at && d.data[end-1] == '\n' {
				end--
			}
			if end > at && d.data[end-1] == '\r' {
				end--
			}
			p.pos = at + i + len("endstream")
		}
		o.isStream, o.stream = true, d.data[at:end]
	}
	p.keyword("endobj")
	o.next = p.pos
	d.at[offset] = o
	return o, nil
}

// length resolves a stream length, which may be stored in another object.
func (d *document) length(o Object) (int, bool) {
	if ref, ok := o.(Ref); ok {
		if d.nesting > 8 {
			return 0, false
		}
		d.nesting++
		defer func() { d.nesting-- }()
		target, err := d.get(ref.Num)
		if err != nil {
			return 0, false
		}
		return asInt(target.value)
	}
	return asInt(o)
}

// get returns the current version of an object.
func (d *document) get(num int) (*object, error) {
	if o, ok := d.objects[num]; ok {
		if o == nil {
			return nil, errMissing
		}
		return o, nil
	}
	pl, ok := d.table[num]
	if !ok || d.nesting > 32 {
		return nil, errMissing
	}
	d.nesting++
	defer func() { d.nesting-- }()
	var o *object
	var err error
	if pl.holder != 0 {
		o, err = d.fromHolder(num, pl)
	} else {
		o, err = d.parseAt(pl.offset)
		if (err != nil || o.num != num) && d.base > 0 {
			o, err = d.parseAt(pl.offset + d.base)
		}
		if err == nil && o.num != num {
			err = errMissing
		}
		// The table points at the wrong place, so use the copy the survey found.
		if alt := d.latest[num]; err != nil && alt != nil {
			o, err = alt, nil
		}
	}
	if err != nil {
		d.objects[num] = nil
		return nil, err
	}
	d.objects[num] = o
	return o, nil
}

func (d *document) holderOf(num int) (*holder, error) {
	if h, ok := d.holders[num]; ok {
		if h == nil {
			return nil, errMissing
		}
		return h, nil
	}
	d.holders[num] = nil
	o, err := d.get(num)
	if err != nil {
		return nil, err
	}
	h, err := d.openHolder(o)
	if err != nil {
		return nil, err
	}
	d.holders[num] = h
	return h, nil
}

func (d *document) openHolder(o *object) (*holder, error) {
	dict, _ := o.value.(*Dict)
	if !o.isStream || dict == nil {
		return nil, errSyntax
	}
	data, err := decode(dict, o.stream)
	if err != nil {
		return nil, err
	}
	count, _ := asInt(dict.Get("N"))
	first, _ := asInt(dict.Get("First"))
	if count < 0 || count > len(data) || first < 0 || first > len(data) {
		return nil, errSyntax
	}
	h := &holder{data: data}
	p := &parser{src: data}
	for i := 0; i < count; i++ {
		num, ok1 := p.integer()
		at, ok2 := p.integer()
		if !ok1 || !ok2 || first+at > len(data) {
			return nil, errSyntax
		}
		h.nums = append(h.nums, num)
		h.starts = append(h.starts, first+at)
	}
	return h, nil
}

// member parses one object out of an object stream.
func (h *holder) member(i int) (*object, error) {
	p := &parser{src: h.data, pos: h.starts[i]}
	p.space()
	o := &object{num: h.nums[i], src: h.data, start: p.pos}
	value, err := p.object()
	if err != nil {
		return nil, err
	}
	o.value, o.end = value, p.pos
	return o, nil
}

func (d *document) fromHolder(num int, pl place) (*object, error) {
	h, err := d.holderOf(pl.holder)
	if err != nil {
		return nil, err
	}
	if pl.index < len(h.nums) && h.nums[pl.index] == num {
		return h.member(pl.index)
	}
	for i, n := range h.nums {
		if n == num {
			return h.member(i)
		}
	}
	return nil, errMissing
}

// survey reads the file from start to end and records every object header,
// including objects that later edits replaced.
func (d *document) survey() {
	pos := d.base
	for pos < len(d.data) {
		i := bytes.Index(d.data[pos:], []byte("obj"))
		if i < 0 {
			return
		}
		at := pos + i
		pos = at + 3
		start, ok := headerStart(d.data, at)
		if !ok {
			continue
		}
		o, err := d.parseAt(start)
		if err != nil {
			continue
		}
		d.spots = append(d.spots, spot{o.num, start, o})
		d.latest[o.num] = o
		if o.next > pos {
			pos = o.next
		}
	}
}

// headerStart walks back from an "obj" keyword to the start of "N G obj".
func headerStart(data []byte, at int) (int, bool) {
	if at+3 < len(data) && !isSpace(data[at+3]) && !isDelimiter(data[at+3]) {
		return 0, false
	}
	i := at - 1
	digits := func() bool {
		end := i
		for i >= 0 && data[i] >= '0' && data[i] <= '9' {
			i--
		}
		return i < end
	}
	blanks := func() bool {
		end := i
		for i >= 0 && isSpace(data[i]) {
			i--
		}
		return i < end
	}
	if !blanks() || !digits() || !blanks() || !digits() {
		return 0, false
	}
	if i >= 0 && !isSpace(data[i]) && !isDelimiter(data[i]) {
		return 0, false
	}
	return i + 1, true
}

// rebuild recreates the object table from the survey when the file's own
// cross-reference data is missing or unusable.
func (d *document) rebuild() {
	d.table = map[int]place{}
	d.objects = map[int]*object{}
	d.holders = map[int]*holder{}
	d.trailers = nil
	for _, s := range d.spots {
		d.table[s.num] = place{offset: s.offset}
		dict, _ := s.obj.value.(*Dict)
		if !s.obj.isStream || dict.Get("Type") != Name("ObjStm") {
			continue
		}
		if h, err := d.openHolder(s.obj); err == nil {
			for i, num := range h.nums {
				d.table[num] = place{offset: -1, holder: s.num, index: i}
			}
		}
	}
	for at := len(d.data); at > 0; {
		at = bytes.LastIndex(d.data[:at], []byte("trailer"))
		if at < 0 {
			break
		}
		p := &parser{src: d.data, pos: at + len("trailer")}
		if v, err := p.object(); err == nil {
			if t, ok := v.(*Dict); ok && t.has("Root") {
				d.trailers = append(d.trailers, t)
			}
		}
	}
	for i := len(d.spots) - 1; i >= 0; i-- {
		dict, _ := d.spots[i].obj.value.(*Dict)
		if dict.Get("Type") == Name("XRef") && dict.has("Root") {
			d.trailers = append(d.trailers, dict)
		}
	}
	if d.catalog() == nil {
		for num := range d.table {
			if o, err := d.get(num); err == nil {
				if dict, _ := o.value.(*Dict); dict.Get("Type") == Name("Catalog") {
					d.trailers = []*Dict{{entries: []entry{{key: "Root", value: Ref{num, o.gen}}}}}
					break
				}
			}
		}
	}
	d.revisions = max(1, bytes.Count(d.data, []byte("%%EOF")))
}

// resolve follows a reference to the value it points at.
func (d *document) resolve(o Object) Object {
	for hops := 0; hops < 16; hops++ {
		ref, ok := o.(Ref)
		if !ok {
			return o
		}
		target, err := d.get(ref.Num)
		if err != nil {
			return nil
		}
		o = target.value
	}
	return nil
}

func (d *document) dict(o Object) *Dict {
	v, _ := d.resolve(o).(*Dict)
	return v
}

// numbers lists the objects the document currently uses, in order.
func (d *document) numbers() []int {
	out := make([]int, 0, len(d.table))
	for num := range d.table {
		out = append(out, num)
	}
	sort.Ints(out)
	return out
}

// pages counts the pages by walking the page tree.
func (d *document) pages() int {
	seen := map[*Dict]bool{}
	var count func(node *Dict, depth int) int
	count = func(node *Dict, depth int) int {
		if node == nil || seen[node] || depth > 64 {
			return 0
		}
		seen[node] = true
		kids, ok := d.resolve(node.Get("Kids")).(Array)
		if !ok {
			if node.Get("Type") == Name("Pages") {
				return 0
			}
			return 1
		}
		total := 0
		for _, kid := range kids {
			total += count(d.dict(kid), depth+1)
		}
		return total
	}
	return count(d.dict(d.catalog().Get("Pages")), 0)
}
