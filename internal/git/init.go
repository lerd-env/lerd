package git

import (
	"fmt"
	"strings"
)

// EnclosingRepo returns the top of the work tree that tracks dir. A folder the
// enclosing repo ignores is not part of it, so it reports no repo.
func EnclosingRepo(dir string) (string, bool) {
	top, err := Output(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", false
	}
	if _, err := Output(dir, "check-ignore", "-q", "."); err == nil {
		return "", false
	}
	return strings.TrimSpace(top), true
}

// Init creates a repository at dir. It refuses when dir already sits inside a
// work tree, since a nested repo would silently shadow the parent one.
func Init(dir string) error {
	if top, ok := EnclosingRepo(dir); ok {
		return fmt.Errorf("%s is already inside a git repository (%s)", dir, top)
	}
	return Run(dir, nil, "init", "-q")
}
