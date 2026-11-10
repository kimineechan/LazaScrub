package scrub

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Item is one file handled while working through a folder.
type Item struct {
	Name   string  // path relative to the folder
	Report *Report // set by Survey
	Result *Result // set by CleanFolder
	Err    error
}

// list returns the supported files in a folder, relative to it and sorted.
func list(folder string, deep bool) ([]string, error) {
	var names []string
	err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == folder {
				return err
			}
			return nil
		}
		if entry.IsDir() {
			if path != folder && !deep {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && Supported(path) {
			if rel, err := filepath.Rel(folder, path); err == nil {
				names = append(names, rel)
			}
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}

// Survey inspects every supported file in a folder and reports each one as
// it is done. With deep set it also goes through the subfolders.
func Survey(folder string, deep bool, each func(done, total int, item Item)) error {
	names, err := list(folder, deep)
	if err != nil {
		return err
	}
	for i, name := range names {
		report, err := Inspect(filepath.Join(folder, name))
		each(i+1, len(names), Item{Name: name, Report: report, Err: err})
	}
	return nil
}

// FreshFolder returns a name for the cleaned copies of a folder: a sibling
// that does not exist yet.
func FreshFolder(folder string) string {
	base := filepath.Clean(folder) + "-clean"
	name := base
	for n := 2; ; n++ {
		if _, err := os.Stat(name); errors.Is(err, fs.ErrNotExist) {
			return name
		}
		name = base + "-" + strconv.Itoa(n)
	}
}

// CleanFolder writes cleaned copies of every supported file in a folder to
// dst, which must not exist yet. Files of other types are not copied.
func CleanFolder(folder, dst string, deep bool, each func(done, total int, item Item)) error {
	names, err := list(folder, deep)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return errors.New("the folder has no files that LazaScrub can clean")
	}
	if err := os.Mkdir(dst, 0700); err != nil {
		return err
	}
	for i, name := range names {
		target := filepath.Join(dst, name)
		err := os.MkdirAll(filepath.Dir(target), 0700)
		var result *Result
		if err == nil {
			result, err = Clean(filepath.Join(folder, name), target)
		}
		each(i+1, len(names), Item{Name: name, Result: result, Err: err})
	}
	return nil
}
