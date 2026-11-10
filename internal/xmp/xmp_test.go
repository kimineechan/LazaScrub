package xmp

import (
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func TestParseListsEveryShapeOfProperty(t *testing.T) {
	found := Parse([]byte(samples.XMP), "XMP")
	want := []struct {
		field, value string
		risk         meta.Risk
	}{
		{"xmp:CreatorTool", "Northlight Studio 4.2 (Windows)", meta.Medium},
		{"xmp:CreateDate", "2026-03-14T09:26:53Z", meta.Medium},
		{"xmpMM:DocumentID", "xmp.did:6f0c2a4e-1d2b-4c55-9a10-3f5a7e2b9c41", meta.Medium},
		{"photoshop:City", "Greenwich", meta.High},
		{"dc:creator", samples.Author, meta.High},
		{"dc:title", "Morning at the observatory", meta.Medium},
		{"dc:subject", "observatory; london", meta.Medium},
		{"xmpMM:History [1]", "stEvt:action=created; stEvt:when=2026-03-14T09:26:53Z; stEvt:softwareAgent=Northlight Studio 4.2 (Windows)", meta.Medium},
	}
	for _, w := range want {
		f, ok := meta.Lookup(found, "XMP", w.field)
		switch {
		case !ok:
			t.Errorf("%s is missing", w.field)
		case f.Value != w.value:
			t.Errorf("%s = %q, want %q", w.field, f.Value, w.value)
		case f.Risk != w.risk:
			t.Errorf("%s risk = %v, want %v", w.field, f.Risk, w.risk)
		}
	}
	if len(found) != 9 {
		t.Errorf("found %d properties, want 9", len(found))
	}
}

func TestParseHandlesNestedAndShorthandStructures(t *testing.T) {
	packet := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:xmpMM="http://ns.adobe.com/xap/1.0/mm/" xmlns:stRef="http://ns.adobe.com/xap/1.0/sType/ResourceRef#" xmlns:Iptc4xmpCore="http://iptc.org/std/Iptc4xmpCore/1.0/xmlns/">
<xmpMM:DerivedFrom stRef:instanceID="xmp.iid:1" stRef:documentID="xmp.did:2"/>
<Iptc4xmpCore:CreatorContactInfo rdf:parseType="Resource"><Iptc4xmpCore:CiEmailWork>alex@example.com</Iptc4xmpCore:CiEmailWork></Iptc4xmpCore:CreatorContactInfo>
</rdf:Description></rdf:RDF></x:xmpmeta>`
	found := Parse([]byte(packet), "XMP")
	if f, ok := meta.Lookup(found, "XMP", "xmpMM:DerivedFrom / stRef:documentID"); !ok || f.Value != "xmp.did:2" {
		t.Errorf("shorthand structure: %+v", found)
	}
	f, ok := meta.Lookup(found, "XMP", "Iptc4xmpCore:CreatorContactInfo / Iptc4xmpCore:CiEmailWork")
	if !ok || f.Value != "alex@example.com" || f.Risk != meta.High {
		t.Errorf("nested structure: %+v", found)
	}
}

func TestParseSurvivesDamage(t *testing.T) {
	for cut := 0; cut < len(samples.XMP); cut += 11 {
		Parse([]byte(samples.XMP[:cut]), "XMP")
	}
	Parse([]byte("\x00\x01\x02 not xml at all <<<>>>"), "XMP")
}
