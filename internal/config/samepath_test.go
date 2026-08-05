package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// caseFoldingVolume reports whether dir sits on a filesystem that treats two
// spellings of one name as the same file. It is the macOS default and not the
// Linux one, and it is a per-volume property rather than a per-OS one, so it is
// asked of the directory the test actually runs in.
func caseFoldingVolume(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "casefold-probe")
	if err := os.Mkdir(probe, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probe)
	_, err := os.Stat(filepath.Join(dir, "CASEFOLD-PROBE"))
	return err == nil
}

func TestSamePath_sameDirectory(t *testing.T) {
	dir := t.TempDir()
	if !SamePath(dir, dir) {
		t.Error("SamePath should hold for one path against itself")
	}
	if !SamePath(dir, dir+string(filepath.Separator)) {
		t.Error("SamePath should ignore a trailing separator")
	}
}

func TestSamePath_symlinkedSpelling(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if !SamePath(real, link) {
		t.Errorf("SamePath(%q, %q) = false, want true for a symlinked spelling", real, link)
	}
}

func TestSamePath_differentDirectories(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if SamePath(a, b) {
		t.Errorf("SamePath(%q, %q) = true, want false for two distinct directories", a, b)
	}
}

// A registry entry whose directory has been deleted still has to compare equal
// to itself, so the string comparison stays as the fallback.
func TestSamePath_missingPathsCompareAsStrings(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "removed")
	if !SamePath(gone, gone) {
		t.Error("SamePath should hold for one missing path against itself")
	}
	if SamePath(gone, gone+"-other") {
		t.Error("SamePath should not hold for two different missing paths")
	}
	if !SamePath("", "") {
		t.Error(`SamePath("", "") should hold`)
	}
	if SamePath("", gone) {
		t.Error("SamePath should not hold for an empty path against a real one")
	}
}

// The case that resolving symlinks cannot answer: one directory, two spellings
// that differ only in case. Skipped where the volume keeps them distinct, since
// there they really are two directories.
func TestSamePath_caseVariantSpelling(t *testing.T) {
	parent := t.TempDir()
	if !caseFoldingVolume(t, parent) {
		t.Skip("volume is case-sensitive; the two spellings are genuinely different directories")
	}
	lower := filepath.Join(parent, "project")
	if err := os.Mkdir(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	upper := filepath.Join(parent, "PROJECT")
	if !SamePath(lower, upper) {
		t.Errorf("SamePath(%q, %q) = false, want true: one directory spelled two ways", lower, upper)
	}
}

// The defect this guards: linking a directory through a differently-cased path
// registered it a second time, because the lookup compared canonical strings and
// resolving symlinks does not fold case.
func TestFindSiteByPath_caseVariantSpelling(t *testing.T) {
	setDataDir(t)
	parent := t.TempDir()
	if !caseFoldingVolume(t, parent) {
		t.Skip("volume is case-sensitive; the two spellings are genuinely different directories")
	}
	lower := filepath.Join(parent, "project")
	if err := os.Mkdir(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "project", Path: lower}); err != nil {
		t.Fatal(err)
	}

	upper := filepath.Join(parent, strings.ToUpper("project"))
	got, err := FindSiteByPath(upper)
	if err != nil {
		t.Fatalf("FindSiteByPath(%q) should find the site registered at %q: %v", upper, lower, err)
	}
	if got.Name != "project" {
		t.Errorf("found %q, want the site registered for that directory", got.Name)
	}
}
