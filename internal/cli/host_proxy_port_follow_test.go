package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestServicePortMappings_appliesPublishedPortOverride proves the host-proxy
// env writer reads the moved port: servicePortMappings must reflect a
// PublishedPort override (set by `lerd service port` or the guard), since that
// override lives in global config, not the preset/quadlet meta the lookups read.
func TestServicePortMappings_appliesPublishedPortOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Default preset, no override → the preset's primary mapping (3306:3306).
	base := servicePortMappings("mysql")
	if len(base) == 0 || base[0] != "3306:3306" {
		t.Fatalf("servicePortMappings(mysql) with no override = %v, want first 3306:3306", base)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Services["mysql"] = config.ServiceConfig{Enabled: true, Port: 3306, PublishedPort: 3307}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	got := servicePortMappings("mysql")
	if len(got) == 0 || got[0] != "3307:3306" {
		t.Errorf("servicePortMappings(mysql) after override = %v, want first 3307:3306 (host side moved, container side kept)", got)
	}
}

// TestRewriteEnvForHostProxy_followsPublishedPortOverride is the end-to-end proof
// that a host-proxy site's .env follows a moved published port: the container DNS
// host collapses to loopback and the DB port maps to the OVERRIDDEN published
// port (3307), not the vacated default (3306) a host server now owns.
func TestRewriteEnvForHostProxy_followsPublishedPortOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Services["mysql"] = config.ServiceConfig{Enabled: true, Port: 3306, PublishedPort: 3307}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	updates := map[string]string{"DB_HOST": "lerd-mysql", "DB_PORT": "3306"}
	rewriteEnvForHostProxy(updates, []string{"mysql"})
	if updates["DB_HOST"] != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want 127.0.0.1 (host-proxy reaches the service on loopback)", updates["DB_HOST"])
	}
	if updates["DB_PORT"] != "3307" {
		t.Errorf("DB_PORT = %q, want 3307 (follows the published-port override)", updates["DB_PORT"])
	}
}
