package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Sections of a checkout's uncommitted work, in the order the dashboard lists them.
const (
	SectionStaged    = "staged"
	SectionUnstaged  = "unstaged"
	SectionUntracked = "untracked"
)

// maxVersionBytes is the most of one side of a file the dashboard's editor is handed.
const maxVersionBytes = 1 << 20

// ChangedFile is one path with git's status letter (A, M, D, T, U, or ? for untracked).
type ChangedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	// Lines added and removed; both stay 0 for a binary file.
	Added   int `json:"added,omitempty"`
	Removed int `json:"removed,omitempty"`
}

// Changes is a checkout's uncommitted work split the way `git status` splits it.
type Changes struct {
	Staged    []ChangedFile `json:"staged"`
	Unstaged  []ChangedFile `json:"unstaged"`
	Untracked []ChangedFile `json:"untracked"`
}

// ReadChanges lists the files in each section of dir's uncommitted work, with
// how many lines each adds and removes.
func ReadChanges(dir string) (Changes, error) {
	var c Changes
	var err error
	if c.Staged, err = countedSection(dir, SectionStaged); err != nil {
		return Changes{}, err
	}
	if c.Unstaged, err = countedSection(dir, SectionUnstaged); err != nil {
		return Changes{}, err
	}
	if c.Untracked, err = countedSection(dir, SectionUntracked); err != nil {
		return Changes{}, err
	}
	return c, nil
}

// countedSection is sectionFiles with line counts. Counting reads every file
// in the section, so it is left to the list and kept out of single-file lookups.
func countedSection(dir, section string) ([]ChangedFile, error) {
	files, err := sectionFiles(dir, section)
	if err != nil {
		return nil, err
	}
	if section == SectionUntracked {
		for i := range files {
			files[i].Added = untrackedLines(filepath.Join(dir, files[i].Path))
		}
		return files, nil
	}
	return files, withNumstat(dir, files, sectionArgs(section))
}

func sectionArgs(section string) []string {
	if section == SectionStaged {
		return []string{"diff", "--cached", "--name-status", "--no-renames", "-z"}
	}
	return []string{"diff", "--name-status", "--no-renames", "-z"}
}

func sectionFiles(dir, section string) ([]ChangedFile, error) {
	switch section {
	case SectionStaged, SectionUnstaged:
		return nameStatus(dir, sectionArgs(section)...)
	case SectionUntracked:
		out, err := Output(dir, "ls-files", "--others", "--exclude-standard", "-z")
		if err != nil {
			return nil, err
		}
		files := []ChangedFile{}
		for _, p := range strings.Split(out, "\x00") {
			if p != "" {
				files = append(files, ChangedFile{Path: p, Status: "?"})
			}
		}
		return files, nil
	}
	return nil, fmt.Errorf("unknown diff section %q", section)
}

// untrackedLines counts what an untracked file would add; a binary, oversized
// or unreadable one counts 0, as git's numstat leaves it uncounted.
func untrackedLines(full string) int {
	b, err := workingFile(full)
	if err != nil || len(b) == 0 || bytes.IndexByte(b, 0) >= 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// withNumstat fills each file's line counts from the same diff run with
// --numstat in place of --name-status.
func withNumstat(dir string, files []ChangedFile, args []string) error {
	num := make([]string, len(args))
	for i, a := range args {
		if a == "--name-status" {
			a = "--numstat"
		}
		num[i] = a
	}
	out, err := Output(dir, num...)
	if err != nil {
		return err
	}
	at := map[string]int{}
	for i, f := range files {
		at[f.Path] = i
	}
	// Each record is "added\tremoved\tpath"; a binary file has "-" for both.
	for _, rec := range strings.Split(out, "\x00") {
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		i, ok := at[parts[2]]
		if !ok {
			continue
		}
		files[i].Added, _ = strconv.Atoi(parts[0])
		files[i].Removed, _ = strconv.Atoi(parts[1])
	}
	return nil
}

func nameStatus(dir string, args ...string) ([]ChangedFile, error) {
	out, err := Output(dir, args...)
	if err != nil {
		return nil, err
	}
	files := []ChangedFile{}
	// A conflicted path comes once as U and again with a plain status; U wins.
	seen := map[string]int{}
	fields := strings.Split(out, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "" {
			continue
		}
		f := ChangedFile{Path: fields[i+1], Status: fields[i][:1]}
		if at, dup := seen[f.Path]; dup {
			if f.Status == "U" {
				files[at] = f
			}
			continue
		}
		seen[f.Path] = len(files)
		files = append(files, f)
	}
	return files, nil
}

// Versions are a file's two sides in a section, for a side-by-side editor.
// Binary, TooLarge and Unreadable leave both sides empty; a side the file is
// missing from is "".
type Versions struct {
	Original string `json:"original"`
	Modified string `json:"modified"`
	Binary   bool   `json:"binary"`
	TooLarge bool   `json:"too_large"`
	// Unreadable is a side git cannot hand over as a file, such as a merge
	// conflict, which has no single index entry, or a submodule.
	Unreadable bool `json:"unreadable"`
}

// FileVersions reads both sides of a file in section: staged compares HEAD with
// the index, unstaged the index with the working tree, untracked nothing with it.
func FileVersions(dir, section, path string) (Versions, error) {
	files, err := sectionFiles(dir, section)
	if err != nil {
		return Versions{}, err
	}
	f, ok := find(files, path)
	if !ok {
		return Versions{}, fmt.Errorf("%q has no %s changes", path, section)
	}
	if f.Status == "U" {
		return Versions{Unreadable: true}, nil
	}
	// git's status letter says which side the file is missing from, so a read
	// that fails anywhere else means it cannot be compared, not that it is gone.
	var from, to side
	switch section {
	case SectionStaged:
		from = side{spec: "HEAD:" + path, absent: f.Status == "A"}
		to = side{spec: ":" + path, absent: f.Status == "D"}
	case SectionUnstaged:
		from = side{spec: ":" + path}
		to = side{tree: true, absent: f.Status == "D"}
	case SectionUntracked:
		from = side{absent: true}
		to = side{tree: true}
	}
	return compare(dir, path, from, to), nil
}

// compare reads both sides of path, refusing what an editor cannot show.
func compare(dir, path string, from, to side) Versions {
	var original, modified []byte
	for _, s := range []struct {
		side side
		into *[]byte
	}{{from, &original}, {to, &modified}} {
		b, err := s.side.read(dir, path)
		if errors.Is(err, errTooLarge) {
			return Versions{TooLarge: true}
		}
		if err != nil {
			return Versions{Unreadable: true}
		}
		*s.into = b
	}
	if bytes.IndexByte(original, 0) >= 0 || bytes.IndexByte(modified, 0) >= 0 {
		return Versions{Binary: true}
	}
	return Versions{Original: string(original), Modified: string(modified)}
}

var errTooLarge = errors.New("file too large to compare")

// side is where one version of a file lives: a git object, or the working tree.
type side struct {
	spec   string
	tree   bool
	absent bool
}

// read checks the size before loading anything, so a database dump sitting in
// the checkout is refused without being read into memory.
func (s side) read(dir, path string) ([]byte, error) {
	if s.absent {
		return nil, nil
	}
	if s.tree {
		return workingFile(filepath.Join(dir, path))
	}
	size, err := Output(dir, "cat-file", "-s", s.spec)
	if err != nil {
		return nil, err
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(size), 10, 64); err != nil || n > maxVersionBytes {
		return nil, errTooLarge
	}
	out, err := Output(dir, "cat-file", "blob", s.spec)
	return []byte(out), err
}

// workingFile reads a symlink as its target path, the way git stores it, so a
// link never hands the dashboard a file outside the checkout.
func workingFile(full string) ([]byte, error) {
	fi, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(full)
		return []byte(target), err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a file", full)
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Bounded even after the size check, for a file still being written.
	b, err := io.ReadAll(io.LimitReader(file, maxVersionBytes+1))
	if err == nil && len(b) > maxVersionBytes {
		return nil, errTooLarge
	}
	return b, err
}

func find(files []ChangedFile, path string) (ChangedFile, bool) {
	for _, f := range files {
		if f.Path == path {
			return f, true
		}
	}
	return ChangedFile{}, false
}
