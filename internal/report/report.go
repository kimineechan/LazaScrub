// Package report renders inspection and cleaning results as plain text and JSON.
package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"lazascrub"
	"lazascrub/internal/meta"
	"lazascrub/internal/scrub"
)

const rule = "------------------------------------------------------------------------"

func header(b *strings.Builder, title string, now time.Time) {
	fmt.Fprintf(b, "%s\r\nMade by LazaScrub %s on %s\r\n%s\r\n\r\n", title, lazascrub.Version(), now.Format("2006-01-02 15:04"), rule)
}

func file(b *strings.Builder, label string, r *scrub.Report) {
	fmt.Fprintf(b, "%-11s%s\r\n", label+":", r.Path)
	fmt.Fprintf(b, "%-11s%s, %s (%d bytes)\r\n", "", r.Format, meta.Size(r.Size), r.Size)
	fmt.Fprintf(b, "%-11sSHA-256 %s\r\n", "", r.SHA256)
}

// group writes the findings of one kind under a heading.
func group(b *strings.Builder, title string, list []meta.Finding, risks bool) {
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(b, "\r\n%s (%d)\r\n%s\r\n", title, len(list), rule)
	for _, f := range list {
		if risks {
			fmt.Fprintf(b, "[%s] ", f.Risk)
		}
		fmt.Fprintf(b, "%s / %s\r\n    %s\r\n", f.Section, f.Field, f.Value)
		if f.Note != "" {
			fmt.Fprintf(b, "    %s\r\n", f.Note)
		}
	}
}

func byRisk(list []meta.Finding, risk meta.Risk) []meta.Finding {
	var out []meta.Finding
	for _, f := range list {
		if f.Risk == risk {
			out = append(out, f)
		}
	}
	return out
}

// Text renders an inspection as a plain-text report.
func Text(r *scrub.Report, now time.Time) string {
	var b strings.Builder
	header(&b, "Metadata report", now)
	file(&b, "File", r)
	fmt.Fprintf(&b, "\r\n%s\r\n", r.Summary())
	group(&b, "HIGH RISK", byRisk(r.Findings, meta.High), false)
	group(&b, "MEDIUM RISK", byRisk(r.Findings, meta.Medium), false)
	group(&b, "LOW RISK", byRisk(r.Findings, meta.Low), false)
	return b.String()
}

// CleanText renders a cleaning result as a plain-text report.
func CleanText(r *scrub.Result, now time.Time) string {
	var b strings.Builder
	header(&b, "Cleaning report", now)
	file(&b, "Original", r.Source)
	file(&b, "Clean copy", r.Output)
	fmt.Fprintf(&b, "\r\n%s\r\n%s\r\n", r.Summary(), Check(r))
	group(&b, "REMOVED", r.Removed, true)
	group(&b, "KEPT ON PURPOSE", r.Kept, true)
	group(&b, "STILL PRESENT", r.Left, true)
	return b.String()
}

// Check says whether the content of the cleaned copy matches the original.
func Check(r *scrub.Result) string {
	if r.Same {
		return r.Proof
	}
	return "The content check did not match. Compare the cleaned copy with the original before relying on it."
}

type jsonFile struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type jsonFinding struct {
	Risk    string `json:"risk"`
	Section string `json:"section"`
	Field   string `json:"field"`
	Value   string `json:"value"`
	Note    string `json:"note,omitempty"`
}

func describe(r *scrub.Report) jsonFile {
	return jsonFile{r.Path, r.Format, r.Size, r.SHA256}
}

func convert(list []meta.Finding) []jsonFinding {
	out := make([]jsonFinding, 0, len(list))
	for _, f := range list {
		out = append(out, jsonFinding{f.Risk.String(), f.Section, f.Field, f.Value, f.Note})
	}
	return out
}

// JSON renders an inspection for other tools to read.
func JSON(r *scrub.Report, now time.Time) []byte {
	out, _ := json.MarshalIndent(struct {
		Tool      string        `json:"tool"`
		Generated string        `json:"generated"`
		File      jsonFile      `json:"file"`
		High      int           `json:"high"`
		Medium    int           `json:"medium"`
		Low       int           `json:"low"`
		Findings  []jsonFinding `json:"findings"`
	}{"LazaScrub " + lazascrub.Version(), now.UTC().Format(time.RFC3339), describe(r),
		r.Count(meta.High), r.Count(meta.Medium), r.Count(meta.Low), convert(r.Findings)}, "", "  ")
	return append(out, '\n')
}

// CleanJSON renders a cleaning result for other tools to read.
func CleanJSON(r *scrub.Result, now time.Time) []byte {
	out, _ := json.MarshalIndent(struct {
		Tool             string        `json:"tool"`
		Generated        string        `json:"generated"`
		Original         jsonFile      `json:"original"`
		CleanCopy        jsonFile      `json:"cleanCopy"`
		ContentUnchanged bool          `json:"contentUnchanged"`
		Removed          []jsonFinding `json:"removed"`
		Kept             []jsonFinding `json:"kept"`
		StillPresent     []jsonFinding `json:"stillPresent"`
	}{"LazaScrub " + lazascrub.Version(), now.UTC().Format(time.RFC3339), describe(r.Source), describe(r.Output),
		r.Same, convert(r.Removed), convert(r.Kept), convert(r.Left)}, "", "  ")
	return append(out, '\n')
}
