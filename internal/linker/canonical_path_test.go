package linker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestFreeSiteName_reusesNameForSymlinkedSpelling(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
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
	if err := config.AddSite(config.Site{Name: "app", Path: appReal}); err != nil {
		t.Fatal(err)
	}

	// Linking the same directory through the symlinked spelling must reuse the
	// existing name, not disambiguate it to app-2 (#930).
	if got := FreeSiteName("app", appLink); got != "app" {
		t.Errorf("FreeSiteName(app, symlinked spelling) = %q, want app", got)
	}
}

// One directory is one site. Linking an already-registered directory under a
// different name used to add a second entry for the same path, which is how a
// project ends up served twice with duplicate vhosts and duplicate workers.
func TestResolve_refusesASecondNameForAnAlreadyLinkedPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "app", Path: dir}); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(dir, testConfig(), CLIPolicy("other", false, nil))
	if err == nil {
		t.Fatal("linking an already-linked directory under a new name was allowed")
	}
	if !strings.Contains(err.Error(), "app") {
		t.Errorf("error should name the existing site, got: %v", err)
	}

	// Re-linking under the same name stays a re-link, not an error.
	if _, err := Resolve(dir, testConfig(), CLIPolicy("app", false, nil)); err != nil {
		t.Errorf("re-linking under the same name failed: %v", err)
	}
}

// caseFoldingVolume reports whether dir sits on a filesystem that treats two
// spellings of one name as the same file. The macOS default, not the Linux one,
// and a per-volume property, so it is asked of the directory in use.
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

// Re-linking one directory through a differently-cased spelling has to land on
// the site that is already there, in the name axis and the domain axis alike.
// Disambiguating either one invents a second identity for a single project.
func TestReLinkThroughCaseVariantSpellingKeepsNameAndDomain(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent := t.TempDir()
	if !caseFoldingVolume(t, parent) {
		t.Skip("volume is case-sensitive; the two spellings are genuinely different directories")
	}
	lower := filepath.Join(parent, "app")
	if err := os.Mkdir(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	upper := filepath.Join(parent, "APP")
	if err := config.AddSite(config.Site{Name: "app", Path: lower, Domains: []string{"app.test"}}); err != nil {
		t.Fatal(err)
	}

	if got := FreeSiteName("app", upper); got != "app" {
		t.Errorf("FreeSiteName through the case variant = %q, want %q", got, "app")
	}
	kept, removed := ResolveDomains([]string{"app.test"}, "app", upper, "test")
	if len(removed) != 0 {
		t.Errorf("ResolveDomains removed %v, want nothing: the domain is this directory's own", removed)
	}
	if len(kept) != 1 || kept[0] != "app.test" {
		t.Errorf("ResolveDomains kept %v, want [app.test]", kept)
	}
}

// The same question asked of the filter directly: a site's own domain is not a
// conflict just because the caller spelled the path differently.
func TestFilterConflictingDomains_ownPathCaseVariant(t *testing.T) {
	parent := t.TempDir()
	if !caseFoldingVolume(t, parent) {
		t.Skip("volume is case-sensitive; the two spellings are genuinely different directories")
	}
	lower := filepath.Join(parent, "app")
	if err := os.Mkdir(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	sites := []config.Site{{Name: "app", Path: lower, Domains: []string{"app.test"}}}

	kept, removed := FilterConflictingDomains([]string{"app.test"}, filepath.Join(parent, "APP"), sites)
	if len(removed) != 0 {
		t.Errorf("removed %v, want nothing: that domain belongs to this very directory", removed)
	}
	if len(kept) != 1 {
		t.Errorf("kept %v, want the domain retained", kept)
	}
}
