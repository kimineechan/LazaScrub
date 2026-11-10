// Package office finds and removes metadata in Word, Excel and PowerPoint
// files, which are ZIP packages of XML parts.
package office

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"

	"lazascrub/internal/gif"
	"lazascrub/internal/jpeg"
	"lazascrub/internal/meta"
	"lazascrub/internal/pdf"
	"lazascrub/internal/png"
	"lazascrub/internal/webp"
)

const (
	maxPart  = 512 << 20
	maxDepth = 3

	// The name Office itself writes when it removes personal information.
	anonymous = "Author"
)

var (
	tagPattern  = regexp.MustCompile(`<[\w:.-]+\b[^<>]*>`)
	attrPattern = regexp.MustCompile(`([\w:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	leafPattern = regexp.MustCompile(`<(?:[\w.-]+:)?([\w.-]+)(?:\s[^<>]*)?>([^<]+)</`)
	customProp  = regexp.MustCompile(`(?s)<property\b([^<>]*)>\s*<vt:[\w]+>([^<]*)</vt:`)
	authorList  = regexp.MustCompile(`(?s)<authors>.*?</authors>`)
	authorItem  = regexp.MustCompile(`<author>([^<]*)</author>`)
)

type label struct {
	name string
	risk meta.Risk
	note string
}

const (
	notePerson = "Names a person who worked on the document."
	noteTyped  = "Text stored in the document properties."
	noteWhen   = "Shows when the document was made, changed or printed."
	noteApp    = "Names the app that made the document, and its version."
	noteStats  = "A statistic Office stores about the document."
)

// Properties in docProps/core.xml and docProps/app.xml.
var properties = map[string]label{
	"creator":              {"Author", meta.High, notePerson},
	"lastModifiedBy":       {"Last modified by", meta.High, notePerson},
	"created":              {"Created", meta.Medium, noteWhen},
	"modified":             {"Modified", meta.Medium, noteWhen},
	"lastPrinted":          {"Last printed", meta.Medium, noteWhen},
	"revision":             {"Revision number", meta.Medium, "How many times the document has been saved."},
	"title":                {"Title", meta.Medium, noteTyped},
	"subject":              {"Subject", meta.Medium, noteTyped},
	"keywords":             {"Keywords", meta.Medium, noteTyped},
	"description":          {"Comments", meta.Medium, noteTyped},
	"category":             {"Category", meta.Medium, noteTyped},
	"contentStatus":        {"Status", meta.Medium, noteTyped},
	"identifier":           {"Identifier", meta.Medium, noteTyped},
	"language":             {"Language", meta.Low, ""},
	"version":              {"Version", meta.Medium, noteTyped},
	"Application":          {"Application", meta.Medium, noteApp},
	"AppVersion":           {"Application version", meta.Medium, noteApp},
	"Company":              {"Company", meta.High, "Names the organisation the document came from."},
	"Manager":              {"Manager", meta.High, notePerson},
	"Template":             {"Template", meta.Medium, "The template the document was based on."},
	"HyperlinkBase":        {"Hyperlink base", meta.High, "A base address or folder for the links in the document."},
	"TotalTime":            {"Editing time (minutes)", meta.Medium, "How long the document was open for editing."},
	"Pages":                {"Pages", meta.Low, noteStats},
	"Words":                {"Words", meta.Low, noteStats},
	"Characters":           {"Characters", meta.Low, noteStats},
	"CharactersWithSpaces": {"Characters with spaces", meta.Low, noteStats},
	"Lines":                {"Lines", meta.Low, noteStats},
	"Paragraphs":           {"Paragraphs", meta.Low, noteStats},
	"Slides":               {"Slides", meta.Low, noteStats},
	"Notes":                {"Notes pages", meta.Low, noteStats},
	"HiddenSlides":         {"Hidden slides", meta.Low, noteStats},
	"MMClips":              {"Media clips", meta.Low, noteStats},
	"PresentationFormat":   {"Presentation format", meta.Low, noteStats},
	"DocSecurity":          {"Security setting", meta.Low, noteStats},
}

// Replacement parts with no properties in them.
const (
	emptyCore = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"></cp:coreProperties>`
	emptyApp = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"></Properties>`
	emptyCustom = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/custom-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"></Properties>`
	emptyPeople = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<w15:people xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"></w15:people>`
)

func open(data []byte) (*zip.Reader, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not an Office document")
	}
	return r, nil
}

// Is reports whether data is a Word, Excel or PowerPoint package.
func Is(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return false
	}
	r, err := open(data)
	if err != nil {
		return false
	}
	return kind(r) != ""
}

// kind names the application the package belongs to.
func kind(r *zip.Reader) string {
	types := false
	app := ""
	for _, f := range r.File {
		switch {
		case f.Name == "[Content_Types].xml":
			types = true
		case strings.HasPrefix(f.Name, "word/"):
			app = "Word document"
		case strings.HasPrefix(f.Name, "xl/"):
			app = "Excel workbook"
		case strings.HasPrefix(f.Name, "ppt/"):
			app = "PowerPoint presentation"
		}
	}
	if !types {
		return ""
	}
	return app
}

// Kind returns "Word document", "Excel workbook" or "PowerPoint presentation".
func Kind(data []byte) string {
	r, err := open(data)
	if err != nil {
		return ""
	}
	return kind(r)
}

func read(f *zip.File) []byte {
	if f.UncompressedSize64 > maxPart {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	data, _ := io.ReadAll(io.LimitReader(rc, maxPart))
	return data
}

// attrs returns the attributes of one XML tag.
func attrs(tag []byte) map[string]string {
	out := map[string]string{}
	for _, m := range attrPattern.FindAllSubmatch(tag, -1) {
		value := m[2]
		if m[2] == nil {
			value = m[3]
		}
		out[string(m[1])] = meta.Line(html.UnescapeString(string(value)))
	}
	return out
}

// tags returns every start tag with the given element name.
func tags(data []byte, element string) [][]byte {
	var out [][]byte
	prefix := []byte("<" + element)
	for _, tag := range tagPattern.FindAll(data, -1) {
		if bytes.HasPrefix(tag, prefix) && (len(tag) == len(prefix) || !isNameByte(tag[len(prefix)])) {
			out = append(out, tag)
		}
	}
	return out
}

func isNameByte(c byte) bool {
	return c == ':' || c == '-' || c == '.' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

type scanner struct {
	out   meta.List
	depth int
}

// Scan lists the metadata in an Office document.
func Scan(data []byte) ([]meta.Finding, error) {
	return scan(data, 0)
}

func scan(data []byte, depth int) ([]meta.Finding, error) {
	r, err := open(data)
	if err != nil {
		return nil, err
	}
	if kind(r) == "" {
		return nil, errors.New("not an Office document")
	}
	s := &scanner{depth: depth}
	parts := map[string]*zip.File{}
	var names []string
	for _, f := range r.File {
		parts[f.Name] = f
		names = append(names, f.Name)
	}
	get := func(name string) []byte {
		if f, ok := parts[name]; ok {
			return read(f)
		}
		return nil
	}

	s.properties(get("docProps/core.xml"))
	s.properties(get("docProps/app.xml"))
	for _, m := range customProp.FindAllSubmatch(get("docProps/custom.xml"), -1) {
		name, value := attrs(m[1])["name"], meta.Line(html.UnescapeString(string(m[2])))
		if name != "" && value != "" {
			s.out.Add("Custom properties", name, value, max(meta.Rate(name, value), meta.Medium),
				"A custom property, often added by a document management system.")
		}
	}
	authors := map[string]int{} // comment authors
	editors := map[string]int{} // tracked-change authors
	deletions, sessions := 0, 0
	for _, name := range names {
		f := parts[name]
		switch {
		case strings.HasPrefix(name, "docProps/thumbnail"):
			s.out.Add("Thumbnail", "Preview image", meta.Size(int64(f.UncompressedSize64)), meta.High,
				"A picture of the first page. It is stored when the document is saved, so it can show an earlier state.")
		case strings.Contains(name, "/printerSettings/"):
			s.printer(read(f))
		case strings.HasSuffix(name, ".rels"):
			s.relationships(name, read(f))
		case name == "word/people.xml" || strings.HasPrefix(name, "xl/persons/") || name == "ppt/authors.xml":
			s.people(read(f))
		case name == "ppt/commentAuthors.xml":
			for _, tag := range tags(read(f), "p:cmAuthor") {
				authors[attrs(tag)["name"]]++
			}
		case strings.HasPrefix(name, "xl/comments") && strings.HasSuffix(name, ".xml"):
			for _, block := range authorList.FindAll(read(f), -1) {
				for _, m := range authorItem.FindAllSubmatch(block, -1) {
					authors[meta.Line(html.UnescapeString(string(m[1])))]++
				}
			}
		case name == "xl/workbook.xml":
			s.workbook(read(f))
		case name == "xl/connections.xml":
			for _, tag := range tags(read(f), "connection") {
				if a := attrs(tag); a["name"] != "" {
					s.out.Keep("Data connections", "Connection", a["name"], meta.High,
						"A link to an outside data source, which can name servers and accounts. It is part of the workbook, so it is not removed.")
				}
			}
		case strings.HasPrefix(name, "ppt/slides/") && strings.HasSuffix(name, ".xml"):
			for _, tag := range tags(read(f), "p:sld") {
				if attrs(tag)["show"] == "0" {
					s.out.Keep("Hidden content", "Hidden slide", name[len("ppt/slides/"):], meta.Medium,
						"A slide that is hidden but still in the file. It is content, so it is not removed.")
				}
			}
		case strings.HasPrefix(name, "word/") && strings.HasSuffix(name, ".xml"):
			body := read(f)
			for _, tag := range tagPattern.FindAll(body, -1) {
				if !bytes.Contains(tag, []byte("w:author")) {
					continue
				}
				who := attrs(tag)["w:author"]
				if bytes.HasPrefix(tag, []byte("<w:comment ")) {
					authors[who]++
				} else {
					editors[who]++
				}
			}
			deletions += bytes.Count(body, []byte("<w:delText"))
			if name == "word/settings.xml" {
				sessions += len(tags(body, "w:rsid"))
			}
		case strings.HasSuffix(name, "vbaProject.bin"):
			s.out.Keep("Active content", "Macros", meta.Size(int64(f.UncompressedSize64))+" of VBA code", meta.Medium,
				"Macros are part of the document, so they are not removed.")
		case strings.HasPrefix(name, "customXml/item") && !strings.Contains(name, "Props"):
			s.out.Keep("Custom XML", "Data part", fmt.Sprintf("%s, %s", name, meta.Size(int64(f.UncompressedSize64))), meta.Medium,
				"Data stored by an add-in or a document management system. It can be tied to the content, so it is not removed.")
		case strings.Contains(name, "/media/") || strings.Contains(name, "/embeddings/"):
			s.embedded(name, read(f))
		}
	}
	s.names("Comments", "Comment author", "comment", authors, "The name shown on comments.")
	s.names("Tracked changes", "Change author", "change", editors, "Tracked changes record who edited what.")
	if deletions > 0 {
		s.out.Keep("Tracked changes", "Deleted text", plural(deletions, "deletion")+" still stored", meta.High,
			"Text that was deleted with change tracking on can still be read. Accept or reject the changes in Word to remove it.")
	}
	if sessions > 0 {
		s.out.Add("Revision IDs", "Editing sessions", plural(sessions, "session ID"), meta.Medium,
			"Random IDs added each time the document is edited. Matching IDs show that two documents share an origin.")
	}
	s.times(r)
	return s.out, nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (s *scanner) properties(data []byte) {
	for _, m := range leafPattern.FindAllSubmatch(data, -1) {
		l, ok := properties[string(m[1])]
		value := meta.Line(html.UnescapeString(string(m[2])))
		if !ok || value == "" {
			continue
		}
		risk := l.risk
		if l.name == "Template" {
			risk = max(risk, meta.Rate("", value))
		}
		s.out.Add("Document properties", l.name, value, risk, l.note)
	}
}

func (s *scanner) names(section, field, noun string, counts map[string]int, note string) {
	list := make([]string, 0, len(counts))
	for name := range counts {
		if name != "" && name != anonymous {
			list = append(list, name)
		}
	}
	sort.Strings(list)
	for _, name := range list {
		s.out.Add(section, field, fmt.Sprintf("%s (%s)", name, plural(counts[name], noun)), meta.High, note)
	}
}

// people reports the accounts listed for comments and co-authoring.
func (s *scanner) people(data []byte) {
	for _, element := range []string{"w15:person", "person", "p188:author"} {
		for i, tag := range tags(data, element) {
			a := attrs(tag)
			name := a["w15:author"]
			if name == "" {
				name = a["displayName"]
			}
			if name == "" {
				name = a["name"]
			}
			account := a["userId"]
			if element == "w15:person" {
				if next := tags(data, "w15:presenceInfo"); i < len(next) {
					account = attrs(next[i])["w15:userId"]
				}
			}
			if name == "" || (name == anonymous && account == "") {
				continue
			}
			value := name
			if account != "" {
				value += " (" + account + ")"
			}
			s.out.Add("People", "Account", value, meta.High,
				"The account of someone who commented on or edited the document. It can include an email address.")
		}
	}
}

func (s *scanner) relationships(name string, data []byte) {
	for _, tag := range tags(data, "Relationship") {
		a := attrs(tag)
		target, external := a["Target"], a["TargetMode"] == "External"
		switch {
		case strings.HasSuffix(a["Type"], "/attachedTemplate"):
			s.out.Add("Document settings", "Attached template", target, max(meta.Rate("", target), meta.Medium),
				"The template file the document was based on. A full path usually contains a user name.")
		case strings.HasSuffix(a["Type"], "/externalLinkPath") || strings.HasSuffix(a["Type"], "/xlPathMissing"):
			s.out.Keep("Links", "Linked workbook", target, max(meta.Rate("", target), meta.Medium),
				"Another workbook that formulas read from. It is kept because removing it would break those formulas.")
		case external && meta.Rate("", target) == meta.High:
			s.out.Keep("Links", "Link to a local file", target, meta.High,
				"A link that points at a file on a computer or network share. It is content, so it is not removed.")
		}
	}
}

func (s *scanner) workbook(data []byte) {
	for _, tag := range tags(data, "x15ac:absPath") {
		if folder := attrs(tag)["url"]; folder != "" {
			s.out.Add("Workbook", "Saved in folder", folder, meta.High,
				"The folder the workbook was last saved in, which usually contains a user name.")
		}
	}
	for _, tag := range tags(data, "sheet") {
		if a := attrs(tag); a["state"] == "hidden" || a["state"] == "veryHidden" {
			s.out.Keep("Hidden content", "Hidden sheet", a["name"], meta.Medium,
				"A sheet that is hidden but still in the file. It is content, so it is not removed.")
		}
	}
}

// printer reads the device name at the start of a Windows printer settings block.
func (s *scanner) printer(data []byte) {
	name := meta.UTF16(data[:min(len(data), 64)], false)
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	name = meta.Line(name)
	if name == "" {
		name = meta.Size(int64(len(data))) + " of printer settings"
	}
	s.out.Add("Printer settings", "Printer", name, meta.High,
		"The printer the document was set up for. The name of a network printer can identify an organisation.")
}

// embedded scans a picture or document stored inside the package.
func (s *scanner) embedded(name string, data []byte) {
	var inner []meta.Finding
	var err error
	label := "Picture " + name
	switch {
	case jpeg.Is(data):
		inner, err = jpeg.Scan(data)
	case png.Is(data):
		inner, err = png.Scan(data)
	case gif.Is(data):
		inner, err = gif.Scan(data)
	case webp.Is(data):
		inner, err = webp.Scan(data)
	case pdf.Is(data):
		inner, err = pdf.Scan(data)
		label = "Document " + name
	case s.depth < maxDepth && Is(data):
		inner, err = scan(data, s.depth+1)
		label = "Document " + name
	case strings.Contains(name, "/embeddings/"):
		s.out.Keep("Embedded objects", "Embedded object", fmt.Sprintf("%s, %s", name, meta.Size(int64(len(data)))), meta.Medium,
			"A file from another app stored inside the document. It can carry metadata of its own and is not cleaned.")
		return
	}
	if err != nil {
		return
	}
	var leaks []meta.Finding
	for _, f := range inner {
		if !f.Kept {
			leaks = append(leaks, f)
		}
	}
	s.out.Nest(label, leaks)
}

// times reports save times recorded for the parts of the package. Office
// writes a fixed date, but other apps record the real time.
func (s *scanner) times(r *zip.Reader) {
	newest := ""
	for _, f := range r.File {
		if f.Modified.Year() > 1980 {
			if stamp := f.Modified.UTC().Format("2006-01-02 15:04:05"); stamp > newest {
				newest = stamp
			}
		}
	}
	if newest != "" {
		s.out.Add("Package", "Part timestamps", "Newest "+newest+" UTC", meta.Medium,
			"Each part of the file records when it was written.")
	}
	if r.Comment != "" {
		s.out.Add("Package", "Archive comment", meta.Line(r.Comment), meta.Medium, "Text stored in the ZIP container.")
	}
}
