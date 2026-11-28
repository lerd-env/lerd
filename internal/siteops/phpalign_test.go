package siteops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

func stubInstalled(t *testing.T, versions ...string) {
	t.Helper()
	prev := php.InstalledForRuntimeFn
	php.InstalledForRuntimeFn = func() ([]string, error) { return versions, nil }
	t.Cleanup(func() { php.InstalledForRuntimeFn = prev })
}

// A branch that requires a newer PHP moves the site onto the newest installed
// version it accepts, so composer install runs on a PHP the lock file allows.
func TestAlignPHPVersion_movesOntoAVersionTheCodeAccepts(t *testing.T) {
	site := phpVersionTestSite(t, func(s *config.Site) { s.PHPVersion = "8.3" })
	stubPHPVersionDeps(t, "8.5", "")
	stubInstalled(t, "8.3", "8.4", "8.5")

	got, err := AlignPHPVersion(site, site.Path)
	if err != nil {
		t.Fatalf("AlignPHPVersion: %v", err)
	}
	if got != "8.5" {
		t.Errorf("moved to %q, want 8.5", got)
	}
	if stored, _ := config.FindSite("app"); stored.PHPVersion != "8.5" {
		t.Errorf("registry = %q, want 8.5", stored.PHPVersion)
	}
}

func TestAlignPHPVersion_leavesAnAcceptedVersionAlone(t *testing.T) {
	site := phpVersionTestSite(t, func(s *config.Site) { s.PHPVersion = "8.3" })
	stubPHPVersionDeps(t, "8.2", "")
	stubInstalled(t, "8.3", "8.5")

	got, err := AlignPHPVersion(site, site.Path)
	if err != nil || got != "" {
		t.Fatalf("got %q, %v; want no move", got, err)
	}
	if stored, _ := config.FindSite("app"); stored.PHPVersion != "8.3" {
		t.Errorf("registry = %q, want 8.3", stored.PHPVersion)
	}
}

func TestAlignPHPVersion_refusesWhenNothingInstalledFits(t *testing.T) {
	site := phpVersionTestSite(t, func(s *config.Site) { s.PHPVersion = "8.3" })
	stubPHPVersionDeps(t, "8.5", "")
	stubInstalled(t, "8.3", "8.4")

	_, err := AlignPHPVersion(site, site.Path)
	var rangeErr *PHPRangeError
	if !errors.As(err, &rangeErr) {
		t.Fatalf("err = %v, want a PHPRangeError", err)
	}
}

// The incoming lock decides, not vendor/: a switch leaves the previous branch's
// platform_check.php in place until composer runs on the new lock.
func TestAlignPHPVersion_readsTheIncomingLockNotVendor(t *testing.T) {
	site := phpVersionTestSite(t, func(s *config.Site) { s.PHPVersion = "8.3" })
	stubPHPVersionDeps(t, "", "")
	alignConstraintsFor = func(s *config.Site) []string {
		return compactConstraints(append(manifestPHPConstraints(s), php.LockPHPConstraints(s.Path)...)...)
	}
	stubInstalled(t, "8.3", "8.4", "8.5")
	putSiteFile(t, site.Path, "composer.json", `{"require":{"php":"^8.2"}}`)
	putSiteFile(t, site.Path, "composer.lock", `{"packages":[{"require":{"php":">=8.4.1"}}]}`)
	putSiteFile(t, site.Path, "vendor/composer/platform_check.php", `if (!(PHP_VERSION_ID >= 80200)) {`)

	got, err := AlignPHPVersion(site, site.Path)
	if err != nil || got != "8.5" {
		t.Fatalf("got %q, %v; want 8.5 from the lock", got, err)
	}
}

// A worktree on a slashed branch is tracked under its sanitized label.
func TestAlignPHPVersion_findsAWorktreeByItsLabel(t *testing.T) {
	site := phpVersionTestSite(t, func(s *config.Site) { s.PHPVersion = "8.3" })
	stubPHPVersionDeps(t, "8.5", "")
	stubInstalled(t, "8.3", "8.5")
	wtPath := filepath.Join(site.Path, "wt", "feature-login")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	origDetect, origDR := detectWorktreesFn, podman.DaemonReloadFn
	t.Cleanup(func() { detectWorktreesFn, podman.DaemonReloadFn = origDetect, origDR })
	podman.DaemonReloadFn = func() error { return nil }
	detectWorktreesFn = func(string, string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Branch: "feature-login", GitBranch: "feature/login", Path: wtPath, Domain: "feature-login.app.test"}}, nil
	}

	got, err := AlignPHPVersion(site, wtPath)
	if err != nil || got != "8.5" {
		t.Fatalf("got %q, %v; want 8.5", got, err)
	}
	if v := config.WorktreePHPVersion(wtPath, "8.3"); v != "8.5" {
		t.Errorf("worktree version = %q, want 8.5", v)
	}
}

func putSiteFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A custom FPM image runs its own PHP and a paused site keeps its landing page,
// so neither is moved.
func TestAlignPHPVersion_leavesCustomFPMAndPausedSitesAlone(t *testing.T) {
	for name, tweak := range map[string]func(*config.Site){
		"custom fpm": func(s *config.Site) { s.Runtime = "fpm-custom" },
		"paused":     func(s *config.Site) { s.Paused = true },
	} {
		t.Run(name, func(t *testing.T) {
			site := phpVersionTestSite(t, func(s *config.Site) { s.PHPVersion = "8.3"; tweak(s) })
			stubPHPVersionDeps(t, "8.5", "")
			stubInstalled(t, "8.3", "8.5")

			if got, err := AlignPHPVersion(site, site.Path); err != nil || got != "" {
				t.Fatalf("got %q, %v; want no move", got, err)
			}
		})
	}
}
