package serviceops

import (
	"os"
	"path/filepath"
	"testing"
)

// The web server is handed each service domain with the port its preset
// serves the web UI on, the same one the nginx vhost proxies to.
func TestDomainRoutes_carryThePresetPort(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(cfgHome, "lerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "services:\n    mailpit:\n        domain: mail.test\n"
	if err := os.WriteFile(filepath.Join(cfgHome, "lerd", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	routes := DomainRoutes()
	if len(routes) != 1 {
		t.Fatalf("routes %+v", routes)
	}
	if r := routes[0]; r.Service != "mailpit" || r.Domain != "mail.test" || r.Port != 8025 || r.CORS {
		t.Errorf("route %+v", r)
	}
}
