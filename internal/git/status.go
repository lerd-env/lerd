package git

import (
	"fmt"
	"strings"
)

// Status is a checkout's working-tree state, as a shell prompt summarises it.
type Status struct {
	Staged     int  `json:"staged"`
	Modified   int  `json:"modified"`
	Untracked  int  `json:"untracked"`
	Conflicted int  `json:"conflicted"`
	Ahead      int  `json:"ahead"`
	Behind     int  `json:"behind"`
	Upstream   bool `json:"upstream"`
	// Publishable is a branch with no upstream that Push can send to origin.
	Publishable bool `json:"publishable"`
}

// ReadStatus runs `git status` in dir and summarises it.
func ReadStatus(dir string) (Status, error) {
	out, err := Output(dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return Status{}, err
	}
	s := ParseStatus(out)
	s.Publishable = !s.Upstream && onBranch(out) && hasOrigin(dir)
	return s, nil
}

// onBranch is a checkout on a branch that has a commit; before the first one
// the branch ref does not exist yet.
func onBranch(porcelain string) bool {
	return !strings.Contains(porcelain, "# branch.head (detached)") &&
		!strings.Contains(porcelain, "# branch.oid (initial)")
}

func hasOrigin(dir string) bool {
	_, err := Output(dir, "remote", "get-url", "origin")
	return err == nil
}

// ParseStatus reads `git status --porcelain=v2 --branch` output. In a changed
// entry's XY field X is the index and Y the working tree, "." meaning untouched.
func ParseStatus(out string) Status {
	var s Status
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			s.Upstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &s.Ahead, &s.Behind)
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			if len(line) < 4 {
				continue
			}
			if line[2] != '.' {
				s.Staged++
			}
			if line[3] != '.' {
				s.Modified++
			}
		case strings.HasPrefix(line, "u "):
			s.Conflicted++
		case strings.HasPrefix(line, "? "):
			s.Untracked++
		}
	}
	return s
}
