// Package xmp reads XMP packets: the RDF/XML metadata that editing apps,
// cameras and phones embed in pictures and documents.
package xmp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"lazascrub/internal/meta"
)

const (
	rdfNS = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	xmlNS = "http://www.w3.org/XML/1998/namespace"
)

type node struct {
	name     xml.Name
	attrs    []xml.Attr
	children []*node
	text     string
}

type parser struct {
	prefix  map[string]string // namespace to the prefix the packet declares for it
	section string
	out     meta.List
}

type rating struct {
	risk meta.Risk
	note string
}

const noteLink = "An ID that links this file to other files saved from the same original."

// Properties whose risk the name alone does not show.
var ratings = map[string]rating{
	"xmp:CreatorTool":             {meta.Medium, "Names the app that created the file."},
	"xmpMM:DocumentID":            {meta.Medium, noteLink},
	"xmpMM:OriginalDocumentID":    {meta.Medium, noteLink},
	"xmpMM:InstanceID":            {meta.Medium, noteLink},
	"xmpMM:DerivedFrom":           {meta.Medium, noteLink},
	"xmpMM:History":               {meta.Medium, "A log of each save, with the app used and the time."},
	"photoshop:DocumentAncestors": {meta.Medium, "IDs of the files this one was built from."},
	"dc:creator":                  {meta.High, "Names the author."},
	"dc:rights":                   {meta.High, "Names the owner of the work."},
	"xmp:Thumbnails":              {meta.High, "A preview image stored as text. It can show an earlier state of the file."},
	"dc:format":                   {meta.Low, ""},
	"xmp:Rating":                  {meta.Low, ""},
	"photoshop:ColorMode":         {meta.Low, ""},
	"photoshop:ICCProfile":        {meta.Low, ""},
}

// Parse lists every property in an XMP packet under the given section name.
func Parse(packet []byte, section string) []meta.Finding {
	p := &parser{prefix: map[string]string{rdfNS: "rdf", xmlNS: "xml"}, section: section}
	if len(packet) > 16<<20 {
		p.out.Add(section, "XMP packet", meta.Size(int64(len(packet)))+", too large to list", meta.Medium, "")
		return p.out
	}
	p.walk(p.tree(packet), false)
	if len(p.out) == 0 && len(bytes.TrimSpace(packet)) > 0 {
		p.out.Add(section, "XMP packet", meta.Size(int64(len(packet)))+" with no readable properties", meta.Low, "")
	}
	return p.out
}

func (p *parser) tree(packet []byte) *node {
	d := xml.NewDecoder(bytes.NewReader(packet))
	d.Strict = false
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	root := &node{}
	stack := []*node{root}
	for count := 0; count < 100000; {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			count++
			n := &node{name: t.Name, attrs: append([]xml.Attr(nil), t.Attr...)}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" {
					if _, ok := p.prefix[a.Value]; !ok {
						p.prefix[a.Value] = a.Name.Local
					}
				}
			}
			top := stack[len(stack)-1]
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			stack[len(stack)-1].text += string(t)
		}
	}
	return root
}

func (p *parser) qname(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	if pre, ok := p.prefix[n.Space]; ok {
		return pre + ":" + n.Local
	}
	if !strings.ContainsAny(n.Space, "/:#") {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// walk finds the property lists, which sit directly inside rdf:RDF.
func (p *parser) walk(n *node, inRDF bool) {
	for _, c := range n.children {
		switch {
		case inRDF:
			p.description(c)
		case c.name.Space == rdfNS && c.name.Local == "RDF":
			p.walk(c, true)
		default:
			p.walk(c, false)
		}
	}
}

func structural(a xml.Attr) bool {
	return a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Space == rdfNS || a.Name.Space == xmlNS
}

func (p *parser) description(n *node) {
	for _, a := range n.attrs {
		if !structural(a) {
			p.emit(p.qname(a.Name), a.Value)
		}
	}
	for _, c := range n.children {
		p.property(p.qname(c.name), c)
	}
}

func attr(n *node, space, local string) string {
	for _, a := range n.attrs {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// container returns the list element of an array property.
func container(n *node) *node {
	for _, c := range n.children {
		if c.name.Space == rdfNS && (c.name.Local == "Seq" || c.name.Local == "Bag" || c.name.Local == "Alt") {
			return c
		}
	}
	return nil
}

func isStruct(n *node) bool {
	if container(n) != nil {
		return false
	}
	if attr(n, rdfNS, "parseType") == "Resource" {
		return true
	}
	for _, a := range n.attrs {
		if !structural(a) {
			return true
		}
	}
	return len(n.children) > 0
}

type field struct{ name, value string }

func (p *parser) fields(n *node) []field {
	var out []field
	source := n
	for _, c := range n.children {
		if c.name.Space == rdfNS && c.name.Local == "Description" {
			source = c
			break
		}
	}
	for _, a := range source.attrs {
		if !structural(a) {
			out = append(out, field{p.qname(a.Name), meta.Line(a.Value)})
		}
	}
	for _, c := range source.children {
		if c.name.Space == rdfNS && c.name.Local == "Description" {
			continue
		}
		out = append(out, field{p.qname(c.name), p.inline(c)})
	}
	return out
}

// inline renders a value of any shape on one line.
func (p *parser) inline(n *node) string {
	if list := container(n); list != nil {
		var parts []string
		for _, li := range list.children {
			if v := p.inline(li); v != "" {
				parts = append(parts, v)
			}
		}
		return strings.Join(parts, "; ")
	}
	if isStruct(n) {
		var parts []string
		for _, f := range p.fields(n) {
			if f.value != "" {
				parts = append(parts, f.name+"="+f.value)
			}
		}
		return strings.Join(parts, "; ")
	}
	if res := attr(n, rdfNS, "resource"); res != "" {
		return res
	}
	return meta.Line(n.text)
}

func (p *parser) property(name string, n *node) {
	list := container(n)
	switch {
	case list != nil:
		simple := true
		for _, li := range list.children {
			if isStruct(li) {
				simple = false
			}
		}
		switch {
		case simple && list.name.Local == "Alt" && len(list.children) > 1:
			for _, li := range list.children {
				label := name
				if lang := attr(li, xmlNS, "lang"); lang != "" && lang != "x-default" {
					label += " [" + lang + "]"
				}
				p.emit(label, p.inline(li))
			}
		case simple:
			p.emit(name, p.inline(n))
		default:
			for i, li := range list.children {
				p.emit(fmt.Sprintf("%s [%d]", name, i+1), p.inline(li))
			}
		}
	case isStruct(n):
		for _, f := range p.fields(n) {
			p.emit(name+" / "+f.name, f.value)
		}
	default:
		p.emit(name, p.inline(n))
	}
}

func (p *parser) emit(name, value string) {
	value = meta.Line(value)
	if value == "" {
		return
	}
	if r := []rune(value); len(r) > 600 {
		value = fmt.Sprintf("%s... (%d characters)", string(r[:200]), len(r))
	}
	risk, note := rate(name)
	p.out.Add(p.section, name, value, risk, note)
}

func rate(name string) (meta.Risk, string) {
	base := name
	if i := strings.IndexAny(base, "[/"); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	if r, ok := ratings[base]; ok {
		return r.risk, r.note
	}
	switch {
	case strings.HasPrefix(base, "crs:"):
		return meta.Low, ""
	case strings.Contains(name, "Region"):
		return meta.High, "Marks faces or areas in the picture, often with the names of the people."
	case strings.HasPrefix(base, "Attrib:"):
		return meta.Medium, "Tracking data added by the service that exported the file."
	}
	return meta.RiskOf(name), ""
}
