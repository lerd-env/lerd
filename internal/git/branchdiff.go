package git

import (
	"fmt"
	"strings"
)

// DiffNameStatus maps each file that differs between the trees of from and to
// to its status letter: A only in to, D only in from, M changed.
func DiffNameStatus(dir, from, to string) (map[string]byte, error) {
	out, err := Output(dir, "diff", "--name-status", "--no-renames", from, to)
	if err != nil {
		return nil, err
	}
	files := map[string]byte{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		status, path, ok := strings.Cut(line, "\t")
		if ok && status != "" {
			files[path] = status[0]
		}
	}
	return files, nil
}

// FileAtRef reports whether path exists in ref's tree.
func FileAtRef(dir, ref, path string) bool {
	_, err := Output(dir, "cat-file", "-e", ref+":"+path)
	return err == nil
}

// AheadBehind counts the commits only from has (ahead) and only to has (behind).
func AheadBehind(dir, from, to string) (ahead, behind int) {
	out, err := Output(dir, "rev-list", "--left-right", "--count", from+"..."+to)
	if err == nil {
		fmt.Sscanf(out, "%d %d", &ahead, &behind)
	}
	return ahead, behind
}

// BranchDates maps local and remote branches to their last commit time, unix seconds.
func BranchDates(dir string) map[string]int64 {
	dates := map[string]int64{}
	out, err := Output(dir, "for-each-ref", "--format=%(refname)%09%(committerdate:unix)", "refs/heads", "refs/remotes")
	if err != nil {
		return dates
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		ref, ts, ok := strings.Cut(line, "\t")
		if !ok || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/remotes/")
		var n int64
		fmt.Sscanf(ts, "%d", &n)
		dates[name] = n
	}
	return dates
}

// DirtyFiles lists the paths with uncommitted work: staged, modified or untracked.
func DirtyFiles(dir string) []string {
	out, err := Output(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil
	}
	var files []string
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		files = append(files, e[3:])
		// A rename or copy is followed by its source path, which is not dirty itself.
		if e[0] == 'R' || e[0] == 'C' {
			i++
		}
	}
	return files
}
