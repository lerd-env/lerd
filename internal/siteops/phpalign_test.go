package siteops

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/php"
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
