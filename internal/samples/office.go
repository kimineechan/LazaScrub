package samples

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"image/jpeg"
	"time"
	"unicode/utf16"
)

// Values the sample Office documents carry.
const (
	Company      = "Example Holdings Ltd"
	TemplatePath = `file:///C:\Users\alex.example\AppData\Roaming\Microsoft\Templates\Budget.dotm`
	SavedIn      = `C:\Users\alex.example\Documents\Finance\`
	Printer      = `\\EXAMPLE-PRINT01\Finance F3`
	Account      = "S::sam.reviewer@example.com::7c1f9a52-0b1e-4c7d-9a3e-2f6b8d4e1a90"
	DeletedText  = "the 2025 figure was overstated"
)

const xmlHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const coreProps = xmlHead + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
	`<dc:title>Budget summary</dc:title><dc:creator>` + Author + `</dc:creator><cp:lastModifiedBy>` + Reviewer + `</cp:lastModifiedBy>` +
	`<cp:revision>7</cp:revision><cp:lastPrinted>2026-03-15T17:40:00Z</cp:lastPrinted>` +
	`<dcterms:created xsi:type="dcterms:W3CDTF">2026-03-14T09:26:00Z</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">2026-03-15T18:02:00Z</dcterms:modified></cp:coreProperties>`

func appProps(app string) string {
	return xmlHead + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Template>Budget.dotm</Template><TotalTime>95</TotalTime><Pages>1</Pages><Words>12</Words><Application>` + app + `</Application>` +
		`<Manager>Jordan Lead</Manager><Company>` + Company + `</Company><AppVersion>16.0000</AppVersion></Properties>`
}

const customProps = xmlHead + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/custom-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
	`<property fmtid="{D5CDD505-2E9C-101B-9397-08002B2CF9AE}" pid="2" name="Client"><vt:lpwstr>Northlight plc</vt:lpwstr></property></Properties>`

func rels(items ...string) string {
	out := xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	for i := 0; i+2 < len(items); i += 3 {
		out += `<Relationship Id="` + items[i] + `" Type="` + items[i+1] + `" Target="` + items[i+2] + `"`
		if len(items[i+2]) > 7 && items[i+2][:8] == "file:///" {
			out += ` TargetMode="External"`
		}
		out += `/>`
	}
	return out + `</Relationships>`
}

const (
	relOffice = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	relPack   = "http://schemas.openxmlformats.org/package/2006/relationships/"
)

type file struct {
	name string
	data []byte
}

// pack writes the parts as a ZIP package, with real save times as other
// office suites record them.
func pack(files []file) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		part, _ := w.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: time.Date(2026, 3, 15, 18, 2, 11, 0, time.UTC)})
		part.Write(f.data)
	}
	w.Close()
	return buf.Bytes()
}

func thumbnail() []byte {
	var out bytes.Buffer
	jpeg.Encode(&out, picture(64, 48), &jpeg.Options{Quality: 60})
	return out.Bytes()
}

func shared(app string) []file {
	return []file{
		{"_rels/.rels", []byte(rels(
			"rId1", relOffice+"officeDocument", map[string]string{"Microsoft Office Word": "word/document.xml", "Microsoft Excel": "xl/workbook.xml", "Microsoft Office PowerPoint": "ppt/presentation.xml"}[app],
			"rId2", relPack+"metadata/core-properties", "docProps/core.xml",
			"rId3", relOffice+"extended-properties", "docProps/app.xml",
			"rId4", relOffice+"custom-properties", "docProps/custom.xml",
			"rId5", relPack+"metadata/thumbnail", "docProps/thumbnail.jpeg"))},
		{"docProps/core.xml", []byte(coreProps)},
		{"docProps/app.xml", []byte(appProps(app))},
		{"docProps/custom.xml", []byte(customProps)},
		{"docProps/thumbnail.jpeg", thumbnail()},
	}
}

func contentTypes(overrides ...string) []byte {
	out := xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/><Default Extension="jpeg" ContentType="image/jpeg"/>` +
		`<Default Extension="png" ContentType="image/png"/><Default Extension="bin" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.printerSettings"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		`<Override PartName="/docProps/custom.xml" ContentType="application/vnd.openxmlformats-officedocument.custom-properties+xml"/>`
	for i := 0; i+1 < len(overrides); i += 2 {
		out += `<Override PartName="` + overrides[i] + `" ContentType="` + overrides[i+1] + `"/>`
	}
	return []byte(out + `</Types>`)
}

const wordNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

// DOCX builds a Word document with properties, a thumbnail, a reviewer's
// comment, tracked changes with deleted text, session IDs, the path of its
// template and an embedded photo with GPS data.
func DOCX() []byte {
	wordType := "application/vnd.openxmlformats-officedocument.wordprocessingml."
	document := xmlHead + `<w:document ` + wordNS + `><w:body>` +
		`<w:p w:rsidR="00A31F5C" w:rsidRDefault="00A31F5C"><w:r><w:t>Budget summary for the board</w:t></w:r></w:p>` +
		`<w:p w:rsidR="00B7204D" w:rsidRDefault="00A31F5C"><w:commentRangeStart w:id="0"/><w:r><w:t xml:space="preserve">Spending rose by 4% </w:t></w:r>` +
		`<w:del w:id="1" w:author="` + Reviewer + `" w:date="2026-03-15T17:58:00Z"><w:r><w:delText>` + DeletedText + `</w:delText></w:r></w:del>` +
		`<w:ins w:id="2" w:author="` + Reviewer + `" w:date="2026-03-15T17:59:00Z"><w:r><w:t>after the correction</w:t></w:r></w:ins>` +
		`<w:commentRangeEnd w:id="0"/><w:r><w:commentReference w:id="0"/></w:r></w:p>` +
		`<w:sectPr w:rsidR="00B7204D"><w:pgSz w:w="11906" w:h="16838"/></w:sectPr></w:body></w:document>`
	comments := xmlHead + `<w:comments ` + wordNS + `><w:comment w:id="0" w:author="` + Reviewer + `" w:date="2026-03-15T17:57:00Z" w:initials="SR">` +
		`<w:p><w:r><w:t>Check this figure before it goes out</w:t></w:r></w:p></w:comment></w:comments>`
	people := xmlHead + `<w15:people xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"><w15:person w15:author="` + Reviewer + `">` +
		`<w15:presenceInfo w15:providerId="AD" w15:userId="` + Account + `"/></w15:person></w15:people>`
	settings := xmlHead + `<w:settings ` + wordNS + `><w:attachedTemplate r:id="rId1"/><w:rsids><w:rsidRoot w:val="00A31F5C"/><w:rsid w:val="00A31F5C"/><w:rsid w:val="00B7204D"/></w:rsids></w:settings>`

	files := []file{{"[Content_Types].xml", contentTypes(
		"/word/document.xml", wordType+"document.main+xml", "/word/comments.xml", wordType+"comments+xml",
		"/word/settings.xml", wordType+"settings+xml", "/word/people.xml", "application/vnd.ms-word.people+xml",
		"/docProps/thumbnail.jpeg", "image/jpeg")}}
	files = append(files, shared("Microsoft Office Word")...)
	return pack(append(files,
		file{"word/document.xml", []byte(document)},
		file{"word/_rels/document.xml.rels", []byte(rels(
			"rId1", relOffice+"comments", "comments.xml", "rId2", relOffice+"settings", "settings.xml",
			"rId3", "http://schemas.microsoft.com/office/2011/relationships/people", "people.xml",
			"rId4", relOffice+"image", "media/image1.jpeg"))},
		file{"word/comments.xml", []byte(comments)},
		file{"word/people.xml", []byte(people)},
		file{"word/settings.xml", []byte(settings)},
		file{"word/_rels/settings.xml.rels", []byte(rels("rId1", relOffice+"attachedTemplate", TemplatePath))},
		file{"word/media/image1.jpeg", JPEG(1)},
	))
}

// XLSX builds a workbook that records the folder it was saved in, a network
// printer, a hidden sheet and a comment author.
func XLSX() []byte {
	sheetType := "application/vnd.openxmlformats-officedocument.spreadsheetml."
	main := `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	workbook := xmlHead + `<workbook ` + main + ` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" mc:Ignorable="x15" xmlns:x15="http://schemas.microsoft.com/office/spreadsheetml/2010/11/main">` +
		`<fileVersion appName="xl" lastEdited="7" lowestEdited="7" rupBuild="27425"/>` +
		`<mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><mc:Choice Requires="x15">` +
		`<x15ac:absPath url="` + SavedIn + `" xmlns:x15ac="http://schemas.microsoft.com/office/spreadsheetml/2010/11/ac"/></mc:Choice></mc:AlternateContent>` +
		`<sheets><sheet name="Summary" sheetId="1" r:id="rId1"/><sheet name="Salaries" sheetId="2" state="hidden" r:id="rId2"/></sheets></workbook>`
	sheet := func(text string, extra string) string {
		return xmlHead + `<worksheet ` + main + `><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + text + `</t></is></c></row></sheetData>` + extra + `</worksheet>`
	}
	comments := xmlHead + `<comments ` + main + `><authors><author>` + Reviewer + `</author></authors><commentList>` +
		`<comment ref="A1" authorId="0"><text><t>Check this figure before it goes out</t></text></comment></commentList></comments>`
	device := make([]byte, 220)
	for i, unit := range utf16.Encode([]rune(Printer)) {
		binary.LittleEndian.PutUint16(device[2*i:], unit)
	}

	files := []file{{"[Content_Types].xml", contentTypes(
		"/xl/workbook.xml", sheetType+"sheet.main+xml", "/xl/worksheets/sheet1.xml", sheetType+"worksheet+xml",
		"/xl/worksheets/sheet2.xml", sheetType+"worksheet+xml", "/xl/comments1.xml", sheetType+"comments+xml",
		"/xl/printerSettings/printerSettings1.bin", sheetType+"printerSettings")}}
	files = append(files, shared("Microsoft Excel")...)
	return pack(append(files,
		file{"xl/workbook.xml", []byte(workbook)},
		file{"xl/_rels/workbook.xml.rels", []byte(rels(
			"rId1", relOffice+"worksheet", "worksheets/sheet1.xml", "rId2", relOffice+"worksheet", "worksheets/sheet2.xml"))},
		file{"xl/worksheets/sheet1.xml", []byte(sheet("Budget summary for the board",
			`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/><pageSetup paperSize="9" orientation="portrait" r:id="rId1"/>`))},
		file{"xl/worksheets/_rels/sheet1.xml.rels", []byte(rels(
			"rId1", relOffice+"printerSettings", "../printerSettings/printerSettings1.bin", "rId2", relOffice+"comments", "../comments1.xml"))},
		file{"xl/worksheets/sheet2.xml", []byte(sheet("Salary bands", ""))},
		file{"xl/comments1.xml", []byte(comments)},
		file{"xl/printerSettings/printerSettings1.bin", device},
	))
}

// PPTX builds a presentation with a comment author, a hidden slide and an
// embedded picture that carries its own metadata.
func PPTX() []byte {
	slideType := "application/vnd.openxmlformats-officedocument.presentationml."
	main := `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	presentation := xmlHead + `<p:presentation ` + main + `><p:sldIdLst><p:sldId id="256" r:id="rId1"/><p:sldId id="257" r:id="rId2"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`
	slide := func(text, show string) string {
		return xmlHead + `<p:sld ` + main + show + `><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
			`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>` +
			`</p:spTree></p:cSld></p:sld>`
	}
	authors := xmlHead + `<p:cmAuthorLst ` + main + `><p:cmAuthor id="0" name="` + Reviewer + `" initials="SR" lastIdx="1" clrIdx="0"/></p:cmAuthorLst>`

	files := []file{{"[Content_Types].xml", contentTypes(
		"/ppt/presentation.xml", slideType+"presentation.main+xml", "/ppt/slides/slide1.xml", slideType+"slide+xml",
		"/ppt/slides/slide2.xml", slideType+"slide+xml", "/ppt/commentAuthors.xml", slideType+"commentAuthors+xml")}}
	files = append(files, shared("Microsoft Office PowerPoint")...)
	return pack(append(files,
		file{"ppt/presentation.xml", []byte(presentation)},
		file{"ppt/_rels/presentation.xml.rels", []byte(rels(
			"rId1", relOffice+"slide", "slides/slide1.xml", "rId2", relOffice+"slide", "slides/slide2.xml",
			"rId3", relOffice+"commentAuthors", "commentAuthors.xml"))},
		file{"ppt/slides/slide1.xml", []byte(slide("Budget summary for the board", ""))},
		file{"ppt/slides/_rels/slide1.xml.rels", []byte(rels("rId1", relOffice+"image", "../media/image1.png"))},
		file{"ppt/slides/slide2.xml", []byte(slide("Salary bands", ` show="0"`))},
		file{"ppt/commentAuthors.xml", []byte(authors)},
		file{"ppt/media/image1.png", PNG()},
	))
}
