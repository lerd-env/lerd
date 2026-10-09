package git

import (
	"fmt"
	"strings"
)

// DiffNameStatus maps each file that differs between the trees of from and to
// to its status letter: A only in to, D only in from, M changed. Names are raw,
// unquoted by -z, so they compare equal to DirtyFiles'.
func DiffNameStatus(dir, from, to string) (map[string]byte, error) {
	out, err := Output(dir, "diff", "--name-status", "--no-renames", "-z", from, to)
	if err != nil {
		return nil, err
	}
	files := map[string]byte{}
	fields := strings.Split(out, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] != "" {
			files[fields[i+1]] = fields[i][0]
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
	out, err := Output(dir, "for-each-ref", "--format=%(refname)%09%(symref)%09%(committerdate:unix)", "refs/heads", "refs/remotes")
	if err != nil {
		return dates
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 || f[1] != "" {
			continue // a symbolic ref such as origin/HEAD only points at a branch
		}
		ref, ts := f[0], f[2]
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
		// A rename or copy is followed by its source: a rename deletes it, a copy leaves it be.
		if (e[0] == 'R' || e[0] == 'C') && i+1 < len(entries) {
			i++
			if e[0] == 'R' {
				files = append(files, entries[i])
			}
		}
	}
	return files
}

// RepoPrefix is dir's path inside its repository, "apps/shop/" for a site
// linked below the root and "" at it; git reports tree paths from the root.
func RepoPrefix(dir string) string {
	out, _ := Output(dir, "rev-parse", "--show-prefix")
	return strings.TrimSpace(out)
}

// Branches lists every local and remote-tracking branch, symbolic refs aside.
func Branches(dir string) (local, remote []string) {
	out, err := Output(dir, "for-each-ref", "--format=%(refname)%09%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, nil
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		ref, symref, _ := strings.Cut(line, "\t")
		switch {
		case symref != "":
		case strings.HasPrefix(ref, "refs/heads/"):
			local = append(local, strings.TrimPrefix(ref, "refs/heads/"))
		case strings.HasPrefix(ref, "refs/remotes/"):
			remote = append(remote, strings.TrimPrefix(ref, "refs/remotes/"))
		}
	}
	return local, remote
}
