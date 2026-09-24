package git

import (
	"fmt"
	"strings"
)

// Status is a checkout's working-tree state, as a shell prompt summarises it.
type Status struct {
	Staged     int `json:"staged"`
	Modified   int `json:"modified"`
	Untracked  int `json:"untracked"`
	Conflicted int `json:"conflicted"`
	Ahead      int `json:"ahead"`
	Behind     int `json:"behind"`
}

// ReadStatus runs `git status` in dir and summarises it.
func ReadStatus(dir string) (Status, error) {
	out, err := Output(dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(out), nil
}

// ParseStatus reads `git status --porcelain=v2 --branch` output. In a changed
// entry's XY field X is the index and Y the working tree, "." meaning untouched.
func ParseStatus(out string) Status {
	var s Status
	for _, line := range strings.Split(out, "\n") {
		switch {
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
