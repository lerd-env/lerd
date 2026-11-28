package siteops

import (
	"fmt"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/php"
)

// AlignPHPVersion moves the checkout at dir, the site's or a worktree's, onto the
// newest installed PHP its code accepts when its current one no longer fits, as
// after switching branch. It returns the version moved to, or "" when none was needed.
func AlignPHPVersion(site *config.Site, dir string) (string, error) {
	if site.IsCustomContainer() || site.IsHostProxy() {
		return "", nil
	}
	// A machine-local pin is the user's explicit choice; it is not second-guessed.
	if owns, _ := config.LocalOverrideOwns(dir, "php_version"); owns {
		return "", nil
	}
	probe := *site
	probe.Path = dir
	constraints := alignConstraintsFor(&probe)
	current, opts := site.PHPVersion, PHPVersionOpts{}
	if dir != site.Path {
		current = config.WorktreePHPVersion(dir, site.PHPVersion)
		label, err := worktreeLabel(site, dir)
		if err != nil {
			return "", err
		}
		opts.Branch = label
	}
	if php.SatisfiesAll(current, constraints...) {
		return "", nil
	}
	best := php.BestInstalledFor(constraints...)
	if best == "" {
		return "", &PHPRangeError{Site: site.Name, Requested: current, Constraints: constraints}
	}
	// Forced because the constraints were read from dir, which for a worktree
	// is not the site path SetSitePHPVersion would check them against.
	opts.Force = true
	if _, err := SetSitePHPVersion(site, best, opts); err != nil {
		return "", err
	}
	return best, nil
}

// worktreeLabel is the sanitized branch label lerd tracks the worktree at dir
// under, which is what a branch-scoped version switch looks it up by.
func worktreeLabel(site *config.Site, dir string) (string, error) {
	worktrees, err := detectWorktreesFn(site.Path, site.PrimaryDomain())
	if err != nil {
		return "", fmt.Errorf("detecting worktrees: %w", err)
	}
	for _, wt := range worktrees {
		if filepath.Clean(wt.Path) == filepath.Clean(dir) {
			return wt.Branch, nil
		}
	}
	return "", fmt.Errorf("no worktree of %s at %s", site.Name, dir)
}
