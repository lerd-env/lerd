package php

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// VersionForDir resolves the PHP version a directory's commands must run on.
// This is the single answer the CLI, the MCP server and the version-scoped
// tools all use, so a command can never exec into a different PHP than the
// container serving the same directory.
//
// A worktree's own pin wins first: a worktree checked out inside its parent
// site matches that site by path, so resolving the site first would ignore the
// pin its vhost was generated from. A registered site's version comes next,
// because lerd link clamps to the framework's supported range and re-detecting
// would undo the clamp. Only then does the project's own configuration apply.
func VersionForDir(dir string) (string, error) {
	if wt, parent, ok := WorktreeRootFor(dir); ok {
		return config.WorktreePHPVersion(wt, parent.PHPVersion), nil
	}
	if site, _ := config.FindSiteByPath(SiteRootFor(dir)); site != nil && site.PHPVersion != "" {
		return site.PHPVersion, nil
	}
	version, err := DetectVersion(dir)
	if err == nil {
		return version, nil
	}
	cfg, cfgErr := config.LoadGlobal()
	if cfgErr != nil {
		return "", fmt.Errorf("cannot detect PHP version: %w", err)
	}
	return cfg.PHP.DefaultVersion, nil
}

// WorktreeRootFor returns the worktree checkout containing dir and the site it
// belongs to. Git writes .git as a file in a worktree and as a directory in the
// main checkout, so the walk stops at the first .git either way.
func WorktreeRootFor(dir string) (string, *config.Site, bool) {
	cur := filepath.Clean(dir)
	for {
		fi, err := os.Stat(filepath.Join(cur, ".git"))
		if err == nil {
			if fi.IsDir() {
				return "", nil, false
			}
			if site, ok := config.ParentSiteForWorktreeDir(cur); ok {
				return cur, site, true
			}
			return "", nil, false
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", nil, false
		}
		cur = parent
	}
}

// SiteRootFor returns the registered site path that contains dir, or dir itself
// if no registered site matches. Commands run from anywhere in a project, so
// this walks up to the project root the site was registered at.
func SiteRootFor(dir string) string {
	reg, err := config.LoadSites()
	if err != nil {
		return dir
	}
	dir = filepath.Clean(dir)
	best := ""
	for _, s := range reg.Sites {
		sitePath := filepath.Clean(s.Path)
		if dir == sitePath || strings.HasPrefix(dir, sitePath+string(filepath.Separator)) {
			// Prefer the longest (most-specific) match.
			if len(sitePath) > len(best) {
				best = sitePath
			}
		}
	}
	if best != "" {
		return best
	}
	return dir
}
