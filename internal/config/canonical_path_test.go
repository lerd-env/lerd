package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPath_resolvesSymlinkSpellings(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if CanonicalPath(link) != CanonicalPath(real) {
		t.Errorf("CanonicalPath(%q)=%q, want it to match CanonicalPath(%q)=%q",
			link, CanonicalPath(link), real, CanonicalPath(real))
	}
}

func TestCanonicalPath_fallsBackWhenUnresolvable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does", "..", "nope")
	if got := CanonicalPath(missing); got != filepath.Clean(missing) {
		t.Errorf("CanonicalPath(%q)=%q, want cleaned %q", missing, got, filepath.Clean(missing))
	}
	if CanonicalPath("") != "" {
		t.Error(`CanonicalPath("") should return ""`)
	}
}

func TestAddSiteStoresCanonicalPathAndFindsBothSpellings(t *testing.T) {
	setDataDir(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	appReal := filepath.Join(real, "app")
	appLink := filepath.Join(link, "app")
	if err := os.MkdirAll(appReal, 0o755); err != nil {
		t.Fatal(err)
	}

	// Register through the symlinked spelling; the stored path must be canonical.
	if err := AddSite(Site{Name: "app", Path: appLink}); err != nil {
		t.Fatal(err)
	}
	s, err := FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if s.Path != CanonicalPath(appReal) {
		t.Errorf("stored path = %q, want canonical %q", s.Path, CanonicalPath(appReal))
	}

	// The same directory is found through the real spelling too.
	if _, err := FindSiteByPath(appReal); err != nil {
		t.Errorf("FindSiteByPath(real spelling) should find the symlink-registered site: %v", err)
	}
}
