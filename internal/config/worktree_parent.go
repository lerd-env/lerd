package config

import (
	"os"
	"path/filepath"
	"strings"
)

// ParentSiteForWorktreeDir returns the registered Site whose .git/worktrees/*
// metadata points at dir as a checkout. Used by the PHP/Node version
// detectors to inherit the parent's pinned version before falling through to
// composer/package.json constraints.
//
// Path-based scan over each site's .git/worktrees/<name>/gitdir file; no git
// CLI invocation, no dependency on the gitpkg package (which would create an
// import cycle for callers like internal/php and internal/node).
func ParentSiteForWorktreeDir(dir string) (*Site, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, false
	}
	reg, err := LoadSites()
	if err != nil {
		return nil, false
	}
	for i := range reg.Sites {
		s := &reg.Sites[i]
		if s.Ignored || s.Path == "" {
			continue
		}
		worktreesDir := filepath.Join(s.Path, ".git", "worktrees")
		entries, err := os.ReadDir(worktreesDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(worktreesDir, e.Name(), "gitdir"))
			if err != nil {
				continue
			}
			gitFile := strings.TrimSpace(string(data))
			if !filepath.IsAbs(gitFile) {
				gitFile = filepath.Join(worktreesDir, e.Name(), gitFile)
			}
			// SamePath, since git records the real path and a shell's cwd may
			// spell it through a symlinked /home.
			if SamePath(filepath.Dir(filepath.Clean(gitFile)), abs) {
				return s, true
			}
		}
	}
	return nil, false
}
