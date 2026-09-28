package config

import (
	"os"
	"path/filepath"
	"testing"
)

// On a loopback runtime lerd rewrites the service host to 127.0.0.1, so the env
// names no container and a site that never wrote a .lerd.yaml matches nothing.
// It then drops out of the sweep that follows a published-port move and keeps
// connecting to the port the service used to be on. The framework's own detect
// rules still recognise it, so they are the fourth way to match.
func TestSitesUsingService_MatchesThroughTheFrameworkDetectRules(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := t.TempDir()
	// What `lerd env` leaves behind on the native runtime: the driver still
	// names the service, the host no longer does.
	env := "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3307\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "perf", Domains: []string{"perf.test"}, Path: dir, Framework: "laravel"}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	sites := SitesUsingService("mysql")
	if len(sites) != 1 || sites[0].Name != "perf" {
		t.Fatalf("expected the loopback-rewritten site to be found, got %v", sites)
	}

	// A service the site's env says nothing about must not match, or every
	// sweep would touch every site.
	if other := SitesUsingService("postgres"); len(other) != 0 {
		t.Fatalf("expected no match for a service the site does not use, got %v", other)
	}
}
