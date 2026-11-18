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
		for _, checkout := range WorktreeCheckouts(s.Path) {
			// SamePath, since git records the real path and a shell's cwd may
			// spell it through a symlinked /home.
			if SamePath(checkout, abs) {
				return s, true
			}
		}
	}
	return nil, false
}

// WorktreeMountDir names the folder to bind-mount so a worktree outside its
// site's own mount can be served: the one holding the checkout, which outlives
// it, so removing a worktree never leaves a mount podman can't start with.
func WorktreeMountDir(sitePath, checkout string) (string, bool) {
	if PathWithin(checkout, sitePath) {
		return "", false
	}
	return filepath.Dir(checkout), true
}

// WorktreeCheckouts lists the checkout directories git records for the site's
// worktrees, read from .git/worktrees/*/gitdir, wherever they live on disk.
func WorktreeCheckouts(sitePath string) []string {
	worktreesDir := filepath.Join(sitePath, ".git", "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil
	}
	var out []string
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
		out = append(out, filepath.Dir(filepath.Clean(gitFile)))
	}
	return out
}
