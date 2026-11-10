// lazascrub-cli inspects and cleans files from a terminal or a script.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lazascrub"
	"lazascrub/internal/report"
	"lazascrub/internal/scrub"
)

const usage = `LazaScrub command-line tool

Usage:
  lazascrub-cli scan [--json] <file>...          List the metadata in each file
  lazascrub-cli clean [--json] <file> [<copy>]   Write a cleaned copy and check it
  lazascrub-cli folder [--subfolders] <folder>   Clean every supported file into a new folder
  lazascrub-cli version

The original files are never changed. Without a name for the copy, clean
writes "<name>-clean" next to the original.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	flags := map[string]bool{}
	var args []string
	for _, a := range os.Args[2:] {
		if strings.HasPrefix(a, "--") {
			flags[a] = true
		} else {
			args = append(args, a)
		}
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = scan(args, flags["--json"])
	case "clean":
		err = clean(args, flags["--json"])
	case "folder":
		err = folder(args, flags["--subfolders"])
	case "version", "--version":
		fmt.Println("LazaScrub", lazascrub.Version())
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazascrub-cli:", err)
		os.Exit(1)
	}
}

func scan(files []string, asJSON bool) error {
	if len(files) == 0 {
		return fmt.Errorf("name at least one file to scan")
	}
	failed := 0
	for _, path := range files {
		r, err := scrub.Inspect(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			failed++
			continue
		}
		if asJSON {
			os.Stdout.Write(report.JSON(r, time.Now()))
		} else {
			fmt.Println(report.Text(r, time.Now()))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d files could not be scanned", failed, len(files))
	}
	return nil
}

func clean(args []string, asJSON bool) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("name the file to clean and, if you like, the copy to write")
	}
	src := args[0]
	dst := strings.TrimSuffix(src, filepath.Ext(src)) + "-clean" + filepath.Ext(src)
	if len(args) == 2 {
		dst = args[1]
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists. Name a different copy", dst)
	}
	r, err := scrub.Clean(src, dst)
	if err != nil {
		return err
	}
	if asJSON {
		os.Stdout.Write(report.CleanJSON(r, time.Now()))
	} else {
		fmt.Println(report.CleanText(r, time.Now()))
	}
	return nil
}

func folder(args []string, deep bool) error {
	if len(args) != 1 {
		return fmt.Errorf("name one folder to clean")
	}
	dst := scrub.FreshFolder(args[0])
	failed := 0
	err := scrub.CleanFolder(args[0], dst, deep, func(done, total int, item scrub.Item) {
		if item.Err != nil {
			failed++
			fmt.Printf("[%d/%d] %s: %v\n", done, total, item.Name, item.Err)
			return
		}
		fmt.Printf("[%d/%d] %s: %s\n", done, total, item.Name, item.Result.Summary())
	})
	if err != nil {
		return err
	}
	fmt.Println("Cleaned copies are in", dst)
	if failed > 0 {
		return fmt.Errorf("%d files could not be cleaned", failed)
	}
	return nil
}
