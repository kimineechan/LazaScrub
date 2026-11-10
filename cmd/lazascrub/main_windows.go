//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lazascrub/internal/meta"
	"lazascrub/internal/report"
	"lazascrub/internal/scrub"
	"lazascrub/internal/winui"
)

const reportFilter = "Text report (*.txt)\x00*.txt\x00JSON data (*.json)\x00*.json\x00\x00"

// Text colours for the first column of a table, as 0x00BBGGRR.
const (
	red   = 0x001E26B3
	amber = 0x0000679A
	grey  = 0x006E6E6E
	green = 0x00377F1A
)

var riskColour = map[meta.Risk]uint32{meta.High: red, meta.Medium: amber, meta.Low: grey}

// fileFilter builds the type list of the Open dialog from the supported extensions.
func fileFilter() string {
	patterns := make([]string, len(scrub.Extensions))
	for i, ext := range scrub.Extensions {
		patterns[i] = "*" + ext
	}
	return "Photos, PDFs and Office files\x00" + strings.Join(patterns, ";") + "\x00All files (*.*)\x00*.*\x00\x00"
}

// detail is the text shown under a table for the selected row.
func detail(f meta.Finding) string {
	text := f.Section + " / " + f.Field + "\r\n" + f.Value
	if f.Note != "" {
		text += "\r\n" + f.Note
	}
	return text
}

func main() {
	w, err := winui.New("LazaScrub | find and remove file metadata", "LazaScrubNativeWindow", 960, 700)
	if err != nil {
		winui.Fatal("LazaScrub", err)
		return
	}
	w.Title("LazaScrub", 28, 20, 430, 42)
	w.Label("Every hidden detail. One report. A clean copy.", 29, 67, 880, 26)
	var nav []uintptr
	for i, label := range []string{"01  Inspect a file", "02  Clean and verify", "03  Clean a folder"} {
		nav = append(nav, w.Section(label, i, 28+i*304, 112, 295, 40))
	}
	status := w.Status("Ready. All work stays on this computer.", 28, 662, 903, 26)
	var jobs []uintptr
	busy := false
	// run does slow work off the window's thread, with the controls disabled until it ends.
	run := func(message string, work func() error, done func()) {
		if busy {
			return
		}
		busy = true
		w.SetText(status, message)
		for _, h := range append(append([]uintptr{}, jobs...), nav...) {
			w.Enable(h, false)
		}
		go func() {
			e := work()
			w.Post(func() {
				busy = false
				for _, h := range append(append([]uintptr{}, jobs...), nav...) {
					w.Enable(h, true)
				}
				if e != nil {
					w.SetText(status, "Nothing was changed.")
					w.Alert("Could not complete operation", sentence(e))
				} else {
					w.SetText(status, "Done.")
					if done != nil {
						done()
					}
				}
			})
		}()
	}
	save := func(title, filter, ext, name string) string {
		path, e := w.FileDialog(title, filter, ext, name, true)
		if e != nil {
			w.Alert("Save a file", e.Error())
		}
		return path
	}
	// saveReport writes a report as text, or as JSON when the chosen name ends in .json.
	saveReport := func(name string, text func() string, data func() []byte) {
		path := save("Save the report", reportFilter, "txt", name)
		if path == "" {
			return
		}
		content := []byte(text())
		if strings.EqualFold(filepath.Ext(path), ".json") {
			content = data()
		}
		run("Saving the report...", func() error { return os.WriteFile(path, content, 0600) }, func() { w.SetText(status, "Saved "+path) })
	}

	// 01  Inspect a file
	w.Page(0)
	w.Heading("See what a file gives away", 28, 177, 700, 30)
	w.Label("Lists every piece of metadata in a photo, PDF or Office document. The file is only read, never changed.", 28, 213, 903, 26)
	w.Label("File", 28, 257, 110, 26)
	inspectPath := w.Edit("", 143, 252, 646, 32)
	inspectSummary := w.Label("Choose a file, or drop one anywhere on this window.", 28, 296, 903, 24)
	var inspected *scrub.Report
	var inspectNote uintptr
	inspectList := w.List(28, 326, 903, 204, []winui.Column{{Title: "Risk", Width: 70}, {Title: "Where", Width: 185}, {Title: "Field", Width: 215}, {Title: "Value", Width: 412}}, func(row int) {
		if inspected != nil && row < len(inspected.Findings) {
			w.SetText(inspectNote, detail(inspected.Findings[row]))
		}
	})
	inspectNote = w.Note("Select a row to read the full value and why it matters.", 28, 538, 903, 72)
	inspect := func(path string) {
		var found *scrub.Report
		run("Reading "+filepath.Base(path)+"...", func() error { var e error; found, e = scrub.Inspect(path); return e }, func() {
			inspected = found
			rows := make([][]string, len(found.Findings))
			tints := make([]uint32, len(found.Findings))
			for i, f := range found.Findings {
				rows[i] = []string{f.Risk.String(), f.Section, f.Field, f.Value}
				tints[i] = riskColour[f.Risk]
			}
			inspectList.Fill(rows, tints)
			w.SetPath(inspectPath, path)
			w.SetText(inspectSummary, found.Summary())
			w.SetText(inspectNote, "Select a row to read the full value and why it matters.")
			w.SetText(status, "Read "+path)
			w.ShowPage(0)
		})
	}
	jobs = append(jobs, w.Button("Browse...", 801, 251, 130, 34, func() {
		path, e := w.FileDialog("Choose a file to inspect", fileFilter(), "", "", false)
		if e != nil {
			w.Alert("Choose a file", e.Error())
		} else if path != "" {
			inspect(path)
		}
	}))
	jobs = append(jobs, w.Button("Save report...", 375, 618, 272, 36, func() {
		if inspected == nil {
			w.Alert("Choose a file", "Choose a file to inspect first.")
			return
		}
		r, now := inspected, time.Now()
		saveReport(filepath.Base(r.Path)+" metadata report.txt", func() string { return report.Text(r, now) }, func() []byte { return report.JSON(r, now) })
	}))

	// 02  Clean and verify
	w.Page(1)
	w.Heading("Make a clean copy and check it", 28, 177, 700, 30)
	w.Label("Removes the metadata, saves the result as a new file and scans it again. The original is never changed.", 28, 213, 903, 26)
	w.Label("Original", 28, 257, 110, 26)
	cleanSource := w.Edit("", 143, 252, 646, 32)
	w.Label("Clean copy", 28, 299, 110, 26)
	cleanOutput := w.Edit("", 143, 294, 646, 32)
	cleanSummary := w.Label("Choose a file to clean. You will be asked where to save the copy.", 28, 334, 903, 44)
	var cleaned *scrub.Result
	var outcome []meta.Finding
	var cleanNote uintptr
	cleanList := w.List(28, 380, 903, 150, []winui.Column{{Title: "Result", Width: 100}, {Title: "Risk", Width: 65}, {Title: "Where", Width: 190}, {Title: "Field", Width: 180}, {Title: "Value before", Width: 347}}, func(row int) {
		if row < len(outcome) {
			w.SetText(cleanNote, detail(outcome[row]))
		}
	})
	cleanNote = w.Note("", 28, 538, 903, 72)
	clean := func(source string) {
		ext := filepath.Ext(source)
		suggested := strings.TrimSuffix(filepath.Base(source), ext) + "-clean" + ext
		output := save("Save the cleaned copy", "Same type as the original (*"+ext+")\x00*"+ext+"\x00\x00", strings.TrimPrefix(ext, "."), suggested)
		if output == "" {
			return
		}
		var result *scrub.Result
		run("Cleaning "+filepath.Base(source)+" and checking the copy...", func() error { var e error; result, e = scrub.Clean(source, output); return e }, func() {
			cleaned = result
			outcome = nil
			var rows [][]string
			var tints []uint32
			for _, group := range []struct {
				label string
				tint  uint32
				list  []meta.Finding
			}{{"Still present", red, result.Left}, {"Removed", green, result.Removed}, {"Kept", grey, result.Kept}} {
				for _, f := range group.list {
					outcome = append(outcome, f)
					rows = append(rows, []string{group.label, f.Risk.String(), f.Section, f.Field, f.Value})
					tints = append(tints, group.tint)
				}
			}
			cleanList.Fill(rows, tints)
			w.SetPath(cleanSource, source)
			w.SetPath(cleanOutput, output)
			w.SetText(cleanSummary, result.Summary()+"\r\n"+report.Check(result))
			w.SetText(cleanNote, "Select a row to read what was removed or why it was kept.")
			w.SetText(status, "Created "+output)
			w.ShowPage(1)
		})
	}
	jobs = append(jobs, w.Button("Browse...", 801, 251, 130, 34, func() {
		path, e := w.FileDialog("Choose a file to clean", fileFilter(), "", "", false)
		if e != nil {
			w.Alert("Choose a file", e.Error())
		} else if path != "" {
			clean(path)
		}
	}))
	jobs = append(jobs, w.Button("Show in folder", 801, 293, 130, 34, func() {
		if cleaned != nil {
			w.OpenFolder(filepath.Dir(cleaned.Output.Path))
		}
	}))
	jobs = append(jobs, w.Button("Save report...", 375, 618, 272, 36, func() {
		if cleaned == nil {
			w.Alert("Clean a file", "Clean a file first, then save its report.")
			return
		}
		r, now := cleaned, time.Now()
		saveReport(filepath.Base(r.Source.Path)+" cleaning report.txt", func() string { return report.CleanText(r, now) }, func() []byte { return report.CleanJSON(r, now) })
	}))
	jobs = append(jobs, w.Button("Clean a copy...", 659, 618, 272, 36, func() {
		if source := w.Text(cleanSource); source != "" {
			clean(source)
		} else {
			w.Alert("Choose a file", "Use Browse to choose the file to clean.")
		}
	}))

	// The first section can hand its file straight to the second.
	w.Page(0)
	jobs = append(jobs, w.Button("Clean a copy...", 659, 618, 272, 36, func() {
		if inspected == nil {
			w.Alert("Choose a file", "Choose a file to inspect first.")
			return
		}
		clean(inspected.Path)
	}))

	// 03  Clean a folder
	w.Page(2)
	w.Heading("Clean every file in a folder", 28, 177, 700, 30)
	w.Label("Writes cleaned copies of the photos, PDFs and Office documents in a folder to a new folder beside it.", 28, 213, 903, 26)
	w.Label("Folder", 28, 257, 110, 26)
	folderPath := w.Edit("", 143, 252, 646, 32)
	var deep uintptr
	folderSummary := w.Line("Choose a folder to see what its files contain.", 28, 326, 903, 24)
	folderList := w.List(28, 356, 903, 254, []winui.Column{{Title: "File", Width: 280}, {Title: "Type", Width: 195}, {Title: "Fields", Width: 60}, {Title: "High risk", Width: 75}, {Title: "Result", Width: 272}}, nil)
	rows := map[string]int{}
	cleanFolder := ""
	survey := func(folder string) {
		var items []scrub.Item
		includeSubfolders := w.Checked(deep)
		run("Looking through the folder...", func() error {
			return scrub.Survey(folder, includeSubfolders, func(done, total int, item scrub.Item) {
				items = append(items, item)
				w.Post(func() { w.SetText(status, fmt.Sprintf("Reading file %d of %d...", done, total)) })
			})
		}, func() {
			table := make([][]string, len(items))
			tints := make([]uint32, len(items))
			rows = map[string]int{}
			fields, high := 0, 0
			for i, item := range items {
				rows[item.Name] = i
				if item.Err != nil {
					table[i] = []string{item.Name, "", "", "", sentence(item.Err)}
					continue
				}
				n, h := len(item.Report.Findings), item.Report.Count(meta.High)
				fields, high = fields+n, high+h
				table[i] = []string{item.Name, item.Report.Format, fmt.Sprint(n), fmt.Sprint(h), ""}
			}
			folderList.Fill(table, tints)
			cleanFolder = ""
			w.SetPath(folderPath, folder)
			if len(items) == 0 {
				w.SetText(folderSummary, "This folder has no photos, PDFs or Office documents.")
			} else {
				w.SetText(folderSummary, fmt.Sprintf("%d files hold %d fields, %d of them high risk.", len(items), fields, high))
			}
			w.SetText(status, "Read "+folder)
			w.ShowPage(2)
		})
	}
	deep = w.Checkbox("Include subfolders", 143, 291, 300, 26, func() {
		if folder := w.Text(folderPath); folder != "" {
			survey(folder)
		}
	})
	jobs = append(jobs, deep)
	jobs = append(jobs, w.Button("Browse...", 801, 251, 130, 34, func() {
		folder, e := w.FolderDialog("Choose a folder to clean")
		if e != nil {
			w.Alert("Choose a folder", e.Error())
		} else if folder != "" {
			survey(folder)
		}
	}))
	jobs = append(jobs, w.Button("Open clean folder", 375, 618, 272, 36, func() {
		if cleanFolder == "" {
			w.Alert("Clean the folder", "Clean the folder first. The cleaned copies go into a new folder beside it.")
			return
		}
		w.OpenFolder(cleanFolder)
	}))
	jobs = append(jobs, w.Button("Clean all files", 659, 618, 272, 36, func() {
		folder := w.Text(folderPath)
		if folder == "" || len(rows) == 0 {
			w.Alert("Choose a folder", "Choose a folder that holds photos, PDFs or Office documents first.")
			return
		}
		target := scrub.FreshFolder(folder)
		index := rows
		done, failed := 0, 0
		includeSubfolders := w.Checked(deep)
		run("Cleaning the folder...", func() error {
			return scrub.CleanFolder(folder, target, includeSubfolders, func(n, total int, item scrub.Item) {
				text := ""
				if item.Err != nil {
					failed++
					text = "Not cleaned: " + sentence(item.Err)
				} else {
					done++
					text = brief(item.Result)
				}
				w.Post(func() {
					if row, ok := index[item.Name]; ok {
						folderList.Set(row, 4, text)
					}
					w.SetText(status, fmt.Sprintf("Cleaning file %d of %d...", n, total))
				})
			})
		}, func() {
			cleanFolder = target
			summary := fmt.Sprintf("Cleaned %d files into %s.", done, target)
			if failed > 0 {
				summary += fmt.Sprintf(" %d could not be cleaned and were left out.", failed)
			}
			w.SetText(folderSummary, summary)
			w.SetText(status, "Created "+target)
		})
	}))

	// A file or folder can be dropped on the window or passed on the command line.
	open := func(path string) {
		if info, e := os.Stat(path); e == nil && info.IsDir() {
			survey(path)
		} else {
			inspect(path)
		}
	}
	w.OnDrop(func(paths []string) { open(paths[0]) })
	w.ShowPage(0)
	if len(os.Args) > 1 {
		w.Post(func() { open(os.Args[1]) })
	}
	w.Run()
}

// brief sums up one cleaned file for a table cell.
func brief(r *scrub.Result) string {
	total := len(r.Removed) + len(r.Kept) + len(r.Left)
	text := fmt.Sprintf("Removed %d of %d", len(r.Removed), total)
	if total == 0 {
		text = "Nothing to remove"
	}
	if len(r.Kept) > 0 {
		text += fmt.Sprintf(", kept %d", len(r.Kept))
	}
	if len(r.Left) > 0 {
		text += fmt.Sprintf(", %d not removed", len(r.Left))
	}
	if !r.Same {
		text += ". Check this copy"
	}
	return text
}

// sentence turns an error into a message that starts with a capital and ends with a full stop.
func sentence(e error) string {
	text := e.Error()
	if text == "" {
		return "Unknown error."
	}
	text = strings.ToUpper(text[:1]) + text[1:]
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	return text
}
