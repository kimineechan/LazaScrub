// Package pdf finds and removes metadata in PDF files. It reads the object
// structure of a document without rendering it.
package pdf

import (
	"bytes"
	"errors"
	"strconv"
)

// The PDF value types. A null is a nil Object, a boolean is a bool, and
// numbers are int64 or float64.
type (
	Name   string
	String []byte
	Array  []Object
	Object any
)

// Ref points at an indirect object.
type Ref struct{ Num, Gen int }

// entry is one key of a dictionary, with its position in the source bytes
// so that it can be cut out without touching anything else.
type entry struct {
	key        Name
	value      Object
	start, end int
}

type Dict struct {
	entries    []entry
	start, end int
}

func (d *Dict) Get(key Name) Object {
	if d == nil {
		return nil
	}
	for _, e := range d.entries {
		if e.key == key {
			return e.value
		}
	}
	return nil
}

func (d *Dict) has(key Name) bool {
	if d == nil {
		return false
	}
	for _, e := range d.entries {
		if e.key == key {
			return true
		}
	}
	return false
}

var errSyntax = errors.New("the PDF structure is damaged")

const maxDepth = 200

type parser struct {
	src   []byte
	pos   int
	depth int
}

func isSpace(c byte) bool {
	return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32
}

func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

// space skips white space and comments.
func (p *parser) space() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case isSpace(c):
			p.pos++
		case c == '%':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' {
				p.pos++
			}
		default:
			return
		}
	}
}

// word reads a run of regular characters.
func (p *parser) word() string {
	start := p.pos
	for p.pos < len(p.src) && !isSpace(p.src[p.pos]) && !isDelimiter(p.src[p.pos]) {
		p.pos++
	}
	return string(p.src[start:p.pos])
}

// keyword consumes k when it is the next token.
func (p *parser) keyword(k string) bool {
	p.space()
	if !bytes.HasPrefix(p.src[p.pos:], []byte(k)) {
		return false
	}
	end := p.pos + len(k)
	if end < len(p.src) && !isSpace(p.src[end]) && !isDelimiter(p.src[end]) {
		return false
	}
	p.pos = end
	return true
}

// integer consumes a whole number when it is the next token.
func (p *parser) integer() (int, bool) {
	p.space()
	start := p.pos
	if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
		p.pos++
	}
	digits := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == digits || p.pos-digits > 18 {
		p.pos = start
		return 0, false
	}
	n, err := strconv.Atoi(string(p.src[start:p.pos]))
	if err != nil {
		p.pos = start
		return 0, false
	}
	return n, true
}

// object reads one value of any type.
func (p *parser) object() (Object, error) {
	p.space()
	if p.pos >= len(p.src) || p.depth > maxDepth {
		return nil, errSyntax
	}
	switch c := p.src[p.pos]; {
	case c == '/':
		p.pos++
		return p.name(), nil
	case c == '(':
		return p.literal()
	case c == '<':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '<' {
			return p.dict()
		}
		return p.hexString()
	case c == '[':
		return p.array()
	case c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.number()
	}
	start := p.pos
	switch p.word() {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	p.pos = start
	return nil, errSyntax
}

func (p *parser) array() (Object, error) {
	p.pos++
	p.depth++
	defer func() { p.depth-- }()
	out := Array{}
	for {
		p.space()
		if p.pos >= len(p.src) {
			return nil, errSyntax
		}
		if p.src[p.pos] == ']' {
			p.pos++
			return out, nil
		}
		v, err := p.object()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (p *parser) dict() (Object, error) {
	d := &Dict{start: p.pos}
	p.pos += 2
	p.depth++
	defer func() { p.depth-- }()
	for {
		p.space()
		if p.pos+1 >= len(p.src) {
			return nil, errSyntax
		}
		if p.src[p.pos] == '>' && p.src[p.pos+1] == '>' {
			p.pos += 2
			d.end = p.pos
			return d, nil
		}
		if p.src[p.pos] != '/' {
			return nil, errSyntax
		}
		start := p.pos
		p.pos++
		key := p.name()
		value, err := p.object()
		if err != nil {
			return nil, err
		}
		d.entries = append(d.entries, entry{key, value, start, p.pos})
	}
}

func (p *parser) name() Name {
	var out []byte
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if isSpace(c) || isDelimiter(c) {
			break
		}
		if c == '#' && p.pos+2 < len(p.src) && isHex(p.src[p.pos+1]) && isHex(p.src[p.pos+2]) {
			out = append(out, unhex(p.src[p.pos+1])<<4|unhex(p.src[p.pos+2]))
			p.pos += 3
			continue
		}
		out = append(out, c)
		p.pos++
	}
	return Name(out)
}

// number reads an integer, a real, or a reference such as "12 0 R".
func (p *parser) number() (Object, error) {
	start := p.pos
	if c := p.src[p.pos]; c == '+' || c == '-' {
		p.pos++
	}
	real := false
	for p.pos < len(p.src) && ((p.src[p.pos] >= '0' && p.src[p.pos] <= '9') || p.src[p.pos] == '.') {
		if p.src[p.pos] == '.' {
			real = true
		}
		p.pos++
	}
	text := string(p.src[start:p.pos])
	if n, err := strconv.ParseInt(text, 10, 64); err == nil && !real {
		if n >= 0 {
			save := p.pos
			if gen, ok := p.integer(); ok && gen >= 0 && p.keyword("R") {
				return Ref{int(n), gen}, nil
			}
			p.pos = save
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		if p.pos == start {
			return nil, errSyntax
		}
		return float64(0), nil
	}
	return f, nil
}

func (p *parser) literal() (Object, error) {
	p.pos++
	depth := 1
	out := String{}
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return out, nil
			}
			out = append(out, c)
		case '\\':
			if p.pos >= len(p.src) {
				return nil, errSyntax
			}
			e := p.src[p.pos]
			p.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if p.pos < len(p.src) && p.src[p.pos] == '\n' {
					p.pos++
				}
			case '\n':
			default:
				if e < '0' || e > '7' {
					out = append(out, e)
					break
				}
				v := int(e - '0')
				for k := 0; k < 2 && p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '7'; k++ {
					v = v*8 + int(p.src[p.pos]-'0')
					p.pos++
				}
				out = append(out, byte(v))
			}
		default:
			out = append(out, c)
		}
	}
	return nil, errSyntax
}

func (p *parser) hexString() (Object, error) {
	p.pos++
	out := String{}
	half := -1
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		switch {
		case c == '>':
			if half >= 0 {
				out = append(out, byte(half<<4))
			}
			return out, nil
		case !isHex(c):
		case half < 0:
			half = int(unhex(c))
		default:
			out = append(out, byte(half<<4)|unhex(c))
			half = -1
		}
	}
	return nil, errSyntax
}

func asInt(o Object) (int, bool) {
	switch v := o.(type) {
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

func asInts(o Object) []int {
	list, _ := o.(Array)
	out := make([]int, 0, len(list))
	for _, v := range list {
		n, ok := asInt(v)
		if !ok {
			return nil
		}
		out = append(out, n)
	}
	return out
}
