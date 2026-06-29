package ui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestBuildServiceResponse_publishedPortAndURL proves the service env surface
// reflects a moved published port: ServiceResponse.Port and the connection URL
// must show where lerd actually listens (the override), not the preset default a
// coexisting host server may own. The status pill renders Port, and the Env tab
// renders ConnectionURL, so both follow a `lerd service port` / guard shift.
func TestBuildServiceResponse_publishedPortAndURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// No override → the preset default host port (mysql 3306), in both the Port
	// field and the host-facing connection URL.
	base := buildServiceResponse("mysql")
	if base.Port != 3306 {
		t.Errorf("mysql Port with no override = %d, want 3306 (preset default)", base.Port)
	}
	if !strings.Contains(base.ConnectionURL, ":3306/") {
		t.Errorf("mysql ConnectionURL with no override = %q, want host port 3306", base.ConnectionURL)
	}

	// Move lerd-mysql to 3307 (host server keeps 3306).
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["mysql"] = config.ServiceConfig{Enabled: true, Port: 3306, PublishedPort: 3307}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	moved := buildServiceResponse("mysql")
	if moved.Port != 3307 {
		t.Errorf("mysql Port after move = %d, want 3307 (override)", moved.Port)
	}
	if !strings.Contains(moved.ConnectionURL, ":3307/") || strings.Contains(moved.ConnectionURL, ":3306/") {
		t.Errorf("mysql ConnectionURL after move = %q, want host port 3307 not 3306", moved.ConnectionURL)
	}
}

// TestBuildServiceResponse_nonCanonicalConfiguredPort guards the case a canonical
// preset default would get wrong: a non-canonical preset version seeds its own
// host port into config (e.g. a fresh postgres 18 → 5418), with no published-port
// override. The pill and connection URL must follow the configured port, not the
// canonical preset default (5432), the same precedence CollectPortChecks uses.
func TestBuildServiceResponse_nonCanonicalConfiguredPort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	// Fresh non-canonical version: configured host port differs from the canonical
	// 5432, and no PublishedPort override is set.
	cfg.Services["postgres"] = config.ServiceConfig{Enabled: true, Port: 5418}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	resp := buildServiceResponse("postgres")
	if resp.Port != 5418 {
		t.Errorf("postgres Port = %d, want 5418 (configured non-canonical port, not canonical 5432)", resp.Port)
	}
	if !strings.Contains(resp.ConnectionURL, ":5418/") || strings.Contains(resp.ConnectionURL, ":5432/") {
		t.Errorf("postgres ConnectionURL = %q, want host port 5418 not the canonical 5432", resp.ConnectionURL)
	}
}
