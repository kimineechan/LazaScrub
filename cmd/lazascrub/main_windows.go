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

// Text colours for the first column of a table.
const (
	red   = winui.Red
	amber = winui.Amber
	grey  = winui.Grey
	green = winui.Green
)

var riskColour = map[meta.Risk]winui.Colour{meta.High: red, meta.Medium: amber, meta.Low: grey}

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
	w.Grip(100)
	w.Frame()
	w.Title("LazaScrub", 32, 24, 430, 40)
	w.Quiet("Every hidden detail. One report. A clean copy.", 33, 66, 880, 22)
	w.Strip(32, 104, 896, 44)
	var nav []*winui.Control
	for i, label := range []string{"01  Inspect a file", "02  Clean and verify", "03  Clean a folder"} {
		nav = append(nav, w.Section(label, i, 36+i*298, 108, 292, 36))
	}
	w.Rule(32, 652, 896)
	status := w.Quiet("Ready. All work stays on this computer.", 32, 662, 896, 24)
	var jobs []*winui.Control
	busy := false
	// run does slow work off the window's thread, with the controls disabled until it ends.
	run := func(message string, work func() error, done func()) {
		if busy {
			return
		}
		busy = true
		status.SetText(message)
		for _, c := range append(append([]*winui.Control{}, jobs...), nav...) {
			c.Enable(false)
		}
		go func() {
			e := work()
			w.Post(func() {
				busy = false
				for _, c := range append(append([]*winui.Control{}, jobs...), nav...) {
					c.Enable(true)
				}
				if e != nil {
					status.SetText("Nothing was changed.")
					w.Alert("Could not complete operation", sentence(e))
				} else {
					status.SetText("Done.")
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
		run("Saving the report...", func() error { return os.WriteFile(path, content, 0600) }, func() { status.SetText("Saved " + path) })
	}

	// 01  Inspect a file
	w.Page(0)
	w.Heading("See what a file gives away", 32, 168, 896, 30)
	w.Label("Lists every piece of metadata in a photo, PDF or Office document. The file is only read, never changed.", 32, 202, 896, 24)
	w.Caption("File", 32, 240, 96, 34)
	inspectPath := w.Path("Choose a file, or drop one anywhere on this window", 132, 240, 656, 34)
	inspectSummary := w.Line("Nothing has been read yet.", 32, 284, 896, 24)
	var inspected *scrub.Report
	var inspectNote *winui.Control
	inspectList := w.Table(32, 314, 896, 198, []winui.Column{{Title: "Risk", Width: 84}, {Title: "Where", Width: 190}, {Title: "Field", Width: 220}, {Title: "Value"}}, func(row int) {
		if inspected != nil && row < len(inspected.Findings) {
			inspectNote.SetText(detail(inspected.Findings[row]))
		}
	})
	inspectNote = w.Box("Select a row to read the full value and why it matters.", 32, 520, 896, 76)
	inspect := func(path string) {
		var found *scrub.Report
		run("Reading "+filepath.Base(path)+"...", func() error { var e error; found, e = scrub.Inspect(path); return e }, func() {
			inspected = found
			rows := make([][]string, len(found.Findings))
			tints := make([]winui.Colour, len(found.Findings))
			for i, f := range found.Findings {
				rows[i] = []string{f.Risk.String(), f.Section, f.Field, f.Value}
				tints[i] = riskColour[f.Risk]
			}
			inspectList.Fill(rows, tints)
			inspectPath.SetText(path)
			inspectSummary.SetText(found.Summary())
			inspectNote.SetText("Select a row to read the full value and why it matters.")
			status.SetText("Read " + path)
			w.ShowPage(0)
		})
	}
	jobs = append(jobs, w.Button("Browse...", 798, 240, 130, 34, func() {
		path, e := w.FileDialog("Choose a file to inspect", fileFilter(), "", "", false)
		if e != nil {
			w.Alert("Choose a file", e.Error())
		} else if path != "" {
			inspect(path)
		}
	}))
	jobs = append(jobs, w.Button("Save report...", 384, 604, 266, 38, func() {
		if inspected == nil {
			w.Alert("Choose a file", "Choose a file to inspect first.")
			return
		}
		r, now := inspected, time.Now()
		saveReport(filepath.Base(r.Path)+" metadata report.txt", func() string { return report.Text(r, now) }, func() []byte { return report.JSON(r, now) })
	}))

	// 02  Clean and verify
	w.Page(1)
	w.Heading("Make a clean copy and check it", 32, 168, 896, 30)
	w.Label("Removes the metadata, saves the result as a new file and scans it again. The original is never changed.", 32, 202, 896, 24)
	w.Caption("Original", 32, 240, 96, 34)
	cleanSource := w.Path("Choose a file to clean", 132, 240, 656, 34)
	w.Caption("Clean copy", 32, 280, 96, 34)
	cleanOutput := w.Path("You will be asked where to save it", 132, 280, 656, 34)
	cleanSummary := w.Label("Nothing has been cleaned yet.", 32, 322, 896, 44)
	var cleaned *scrub.Result
	var outcome []meta.Finding
	var cleanNote *winui.Control
	cleanList := w.Table(32, 368, 896, 144, []winui.Column{{Title: "Result", Width: 130}, {Title: "Risk", Width: 80}, {Title: "Where", Width: 180}, {Title: "Field", Width: 190}, {Title: "Value before"}}, func(row int) {
		if row < len(outcome) {
			cleanNote.SetText(detail(outcome[row]))
		}
	})
	cleanNote = w.Box("", 32, 520, 896, 76)
	clean := func(source string) {
		ext := filepath.Ext(source)
		// The dialog opens beside the original, with a name that tells the copy apart.
		suggested := strings.TrimSuffix(source, ext) + "-clean" + ext
		output := save("Save the cleaned copy", "Same type as the original (*"+ext+")\x00*"+ext+"\x00\x00", strings.TrimPrefix(ext, "."), suggested)
		if output == "" {
			return
		}
		var result *scrub.Result
		run("Cleaning "+filepath.Base(source)+" and checking the copy...", func() error { var e error; result, e = scrub.Clean(source, output); return e }, func() {
			cleaned = result
			outcome = nil
			var rows [][]string
			var tints []winui.Colour
			for _, group := range []struct {
				label string
				tint  winui.Colour
				list  []meta.Finding
			}{{"Still present", red, result.Left}, {"Removed", green, result.Removed}, {"Kept", grey, result.Kept}} {
				for _, f := range group.list {
					outcome = append(outcome, f)
					rows = append(rows, []string{group.label, f.Risk.String(), f.Section, f.Field, f.Value})
					tints = append(tints, group.tint)
				}
			}
			cleanList.Fill(rows, tints)
			cleanSource.SetText(source)
			cleanOutput.SetText(output)
			cleanSummary.SetText(result.Summary() + "\r\n" + report.Check(result))
			cleanNote.SetText("Select a row to read what was removed or why it was kept.")
			status.SetText("Created " + output)
			w.ShowPage(1)
		})
	}
	jobs = append(jobs, w.Button("Browse...", 798, 240, 130, 34, func() {
		path, e := w.FileDialog("Choose a file to clean", fileFilter(), "", "", false)
		if e != nil {
			w.Alert("Choose a file", e.Error())
		} else if path != "" {
			clean(path)
		}
	}))
	jobs = append(jobs, w.Button("Show in folder", 798, 280, 130, 34, func() {
		if cleaned != nil {
			w.OpenFolder(filepath.Dir(cleaned.Output.Path))
		}
	}))
	jobs = append(jobs, w.Button("Save report...", 384, 604, 266, 38, func() {
		if cleaned == nil {
			w.Alert("Clean a file", "Clean a file first, then save its report.")
			return
		}
		r, now := cleaned, time.Now()
		saveReport(filepath.Base(r.Source.Path)+" cleaning report.txt", func() string { return report.CleanText(r, now) }, func() []byte { return report.CleanJSON(r, now) })
	}))
	again := w.Button("Clean a copy...", 662, 604, 266, 38, func() {
		if source := cleanSource.Text(); source != "" {
			clean(source)
		} else {
			w.Alert("Choose a file", "Use Browse to choose the file to clean.")
		}
	})
	again.Lead(true)
	jobs = append(jobs, again)

	// The first section can hand its file straight to the second.
	w.Page(0)
	next := w.Button("Clean a copy...", 662, 604, 266, 38, func() {
		if inspected == nil {
			w.Alert("Choose a file", "Choose a file to inspect first.")
			return
		}
		clean(inspected.Path)
	})
	next.Lead(true)
	jobs = append(jobs, next)

	// 03  Clean a folder
	w.Page(2)
	w.Heading("Clean every file in a folder", 32, 168, 896, 30)
	w.Label("Writes cleaned copies of the photos, PDFs and Office documents in a folder to a new folder beside it.", 32, 202, 896, 24)
	w.Caption("Folder", 32, 240, 96, 34)
	folderPath := w.Path("Choose a folder, or drop one anywhere on this window", 132, 240, 656, 34)
	var deep *winui.Control
	folderSummary := w.Line("Choose a folder to see what its files contain.", 32, 316, 896, 24)
	folderList := w.Table(32, 346, 896, 250, []winui.Column{{Title: "File", Width: 270}, {Title: "Type", Width: 190}, {Title: "Fields", Width: 80}, {Title: "High risk", Width: 100}, {Title: "Result"}}, nil)
	rows := map[string]int{}
	cleanFolder := ""
	survey := func(folder string) {
		var items []scrub.Item
		includeSubfolders := deep.On()
		run("Looking through the folder...", func() error {
			return scrub.Survey(folder, includeSubfolders, func(done, total int, item scrub.Item) {
				items = append(items, item)
				w.Post(func() { status.SetText(fmt.Sprintf("Reading file %d of %d...", done, total)) })
			})
		}, func() {
			table := make([][]string, len(items))
			tints := make([]winui.Colour, len(items))
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
			folderPath.SetText(folder)
			if len(items) == 0 {
				folderSummary.SetText("This folder has no photos, PDFs or Office documents.")
			} else {
				folderSummary.SetText(fmt.Sprintf("%d files hold %d fields, %d of them high risk.", len(items), fields, high))
			}
			status.SetText("Read " + folder)
			w.ShowPage(2)
		})
	}
	deep = w.Switch("Include subfolders", 132, 282, 300, 26, func() {
		if folder := folderPath.Text(); folder != "" {
			survey(folder)
		}
	})
	jobs = append(jobs, deep)
	jobs = append(jobs, w.Button("Browse...", 798, 240, 130, 34, func() {
		folder, e := w.FolderDialog("Choose a folder to clean")
		if e != nil {
			w.Alert("Choose a folder", e.Error())
		} else if folder != "" {
			survey(folder)
		}
	}))
	jobs = append(jobs, w.Button("Open clean folder", 384, 604, 266, 38, func() {
		if cleanFolder == "" {
			w.Alert("Clean the folder", "Clean the folder first. The cleaned copies go into a new folder beside it.")
			return
		}
		w.OpenFolder(cleanFolder)
	}))
	all := w.Button("Clean all files", 662, 604, 266, 38, nil)
	all.Lead(true)
	jobs = append(jobs, all)
	all.Do(func() {
		folder := folderPath.Text()
		if folder == "" || len(rows) == 0 {
			w.Alert("Choose a folder", "Choose a folder that holds photos, PDFs or Office documents first.")
			return
		}
		target := scrub.FreshFolder(folder)
		index := rows
		done, failed := 0, 0
		includeSubfolders := deep.On()
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
						folderList.SetCell(row, 4, text)
					}
					status.SetText(fmt.Sprintf("Cleaning file %d of %d...", n, total))
				})
			})
		}, func() {
			cleanFolder = target
			summary := fmt.Sprintf("Cleaned %d files into %s.", done, target)
			if failed > 0 {
				summary += fmt.Sprintf(" %d could not be cleaned and were left out.", failed)
			}
			folderSummary.SetText(summary)
			status.SetText("Created " + target)
		})
	})

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
