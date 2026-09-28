package config

import (
	"os"
	"path/filepath"
	"testing"
)

// On a loopback runtime lerd rewrites the service host to 127.0.0.1, so the env
// names no container and a site that never wrote a .lerd.yaml matched nothing.
// It then dropped out of the sweep that follows a published-port move and kept
// connecting to the port the service used to be on.
func TestSitesUsingService_MatchesWhatLerdEnvWired(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := t.TempDir()
	env := "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3307\nREDIS_HOST=127.0.0.1\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "perf", Domains: []string{"perf.test"}, Path: dir, Framework: "laravel"}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if got := SitesUsingService("mysql"); len(got) != 0 {
		t.Fatalf("expected no match before lerd env recorded anything, got %v", got)
	}

	if err := SetSiteWiredServices("perf", []string{"mysql"}); err != nil {
		t.Fatalf("SetSiteWiredServices: %v", err)
	}
	if got := SitesUsingService("mysql"); len(got) != 1 || got[0].Name != "perf" {
		t.Fatalf("expected the loopback-rewritten site to be found, got %v", got)
	}
	// The env still mentions redis, but lerd did not wire it, so it is not the
	// site's service.
	if got := SitesUsingService("redis"); len(got) != 0 {
		t.Fatalf("expected no match for a service lerd did not wire, got %v", got)
	}
}

// A run that wires a different set replaces the record rather than adding to
// it, so a site moved off a service stops counting as its user.
func TestSetSiteWiredServices_ReplacesTheRecord(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := AddSite(Site{Name: "perf", Domains: []string{"perf.test"}, Path: t.TempDir()}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	if err := SetSiteWiredServices("perf", []string{"redis", "mysql"}); err != nil {
		t.Fatal(err)
	}
	if err := SetSiteWiredServices("perf", []string{"postgres"}); err != nil {
		t.Fatal(err)
	}
	invalidateSitesCache()
	s, err := FindSite("perf")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.WiredServices) != 1 || s.WiredServices[0] != "postgres" {
		t.Fatalf("WiredServices = %v, want [postgres]", s.WiredServices)
	}

	if err := SetSiteWiredServices("ghost", nil); err == nil {
		t.Fatal("expected an error for a site that is not registered")
	}
}
