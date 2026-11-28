package siteops

import (
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/php"
)

// AlignPHPVersion moves the checkout at dir, the site's own or one of its
// worktrees, onto the newest installed PHP its code accepts when the version it
// runs on no longer does, as after switching to a branch that requires a newer
// one. It returns the version moved to, or "" when none was needed.
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
	constraints := phpConstraintsFor(&probe)
	current, opts := site.PHPVersion, PHPVersionOpts{}
	if dir != site.Path {
		current = config.WorktreePHPVersion(dir, site.PHPVersion)
		branch, _ := gitpkg.Output(dir, "symbolic-ref", "--short", "HEAD")
		opts.Branch = strings.TrimSpace(branch)
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
