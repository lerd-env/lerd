package siteinfo

import (
	"os"
	"path/filepath"
	"testing"
)

// sqlite has no preset, no container and nothing to install, so a site page
// rendering it as a service could only offer to install something that cannot
// exist. Projects written before lerd stopped recording it still carry the
// entry, so it is dropped where it is found.
func TestEnrichServices_sqliteIsNotListedAsAService(t *testing.T) {
	installQuadlets(t, "mysql")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - sqlite\n  - mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := &EnrichedSite{Path: dir}
	e.enrichServices()

	for _, s := range e.Services {
		if s == "sqlite" {
			t.Errorf("services = %v, want sqlite left out", e.Services)
		}
	}
	found := false
	for _, s := range e.Services {
		if s == "mysql" {
			found = true
		}
	}
	if !found {
		t.Errorf("services = %v, want the real service still listed", e.Services)
	}
}

// A project whose only picked database is sqlite lists no services rather than
// one that cannot be installed.
func TestEnrichServices_sqliteOnlyProjectListsNothing(t *testing.T) {
	installQuadlets(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := &EnrichedSite{Path: dir}
	e.enrichServices()

	if len(e.Services) != 0 {
		t.Errorf("services = %v, want none", e.Services)
	}
}
