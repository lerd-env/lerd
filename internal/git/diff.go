package git

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
}

// Changes is a checkout's uncommitted work split the way `git status` splits it.
type Changes struct {
	Staged    []ChangedFile `json:"staged"`
	Unstaged  []ChangedFile `json:"unstaged"`
	Untracked []ChangedFile `json:"untracked"`
}

// ReadChanges lists the files in each section of dir's uncommitted work.
func ReadChanges(dir string) (Changes, error) {
	var c Changes
	var err error
	if c.Staged, err = sectionFiles(dir, SectionStaged); err != nil {
		return Changes{}, err
	}
	if c.Unstaged, err = sectionFiles(dir, SectionUnstaged); err != nil {
		return Changes{}, err
	}
	if c.Untracked, err = sectionFiles(dir, SectionUntracked); err != nil {
		return Changes{}, err
	}
	return c, nil
}

func sectionFiles(dir, section string) ([]ChangedFile, error) {
	switch section {
	case SectionStaged:
		return nameStatus(dir, "diff", "--cached", "--name-status", "--no-renames", "-z")
	case SectionUnstaged:
		return nameStatus(dir, "diff", "--name-status", "--no-renames", "-z")
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

func nameStatus(dir string, args ...string) ([]ChangedFile, error) {
	out, err := Output(dir, args...)
	if err != nil {
		return nil, err
	}
	files := []ChangedFile{}
	fields := strings.Split(out, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] != "" {
			files = append(files, ChangedFile{Path: fields[i+1], Status: fields[i][:1]})
		}
	}
	return files, nil
}

// Versions are a file's two sides in a section, for a side-by-side editor.
// Binary and TooLarge leave both sides empty; a side the file is missing from is "".
type Versions struct {
	Original string `json:"original"`
	Modified string `json:"modified"`
	Binary   bool   `json:"binary"`
	TooLarge bool   `json:"too_large"`
}

// FileVersions reads both sides of a file in section: staged compares HEAD with
// the index, unstaged the index with the working tree, untracked nothing with it.
func FileVersions(dir, section, path string) (Versions, error) {
	files, err := sectionFiles(dir, section)
	if err != nil {
		return Versions{}, err
	}
	if !listed(files, path) {
		return Versions{}, fmt.Errorf("%q has no %s changes", path, section)
	}
	var original, modified []byte
	switch section {
	case SectionStaged:
		original, modified = blob(dir, "HEAD:"+path), blob(dir, ":"+path)
	case SectionUnstaged:
		original, modified = blob(dir, ":"+path), workingFile(dir, path)
	case SectionUntracked:
		modified = workingFile(dir, path)
	}
	if len(original) > maxVersionBytes || len(modified) > maxVersionBytes {
		return Versions{TooLarge: true}, nil
	}
	if bytes.IndexByte(original, 0) >= 0 || bytes.IndexByte(modified, 0) >= 0 {
		return Versions{Binary: true}, nil
	}
	return Versions{Original: string(original), Modified: string(modified)}, nil
}

// blob is the object at spec, or nil where the file is not on that side
// (added, deleted, or no commit yet).
func blob(dir, spec string) []byte {
	out, err := Output(dir, "cat-file", "blob", spec)
	if err != nil {
		return nil
	}
	return []byte(out)
}

// workingFile reads a symlink as its target path, the way git stores it, so a
// link never hands the dashboard a file outside the checkout.
func workingFile(dir, path string) []byte {
	full := filepath.Join(dir, path)
	if fi, err := os.Lstat(full); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(full)
		return []byte(target)
	}
	b, _ := os.ReadFile(full)
	return b
}

func listed(files []ChangedFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}
