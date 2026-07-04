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

// TestBuildServiceResponse_dashboardFollowsPublishedPort proves the dashboard URL
// tracks a moved published port. The left-rail launcher and the iframe overlay
// both open ServiceResponse.Dashboard verbatim, so a `lerd service port` move must
// re-point it the same way ConnectionURL is, or the dashboard opens the old port.
func TestBuildServiceResponse_dashboardFollowsPublishedPort(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// No override → the preset default dashboard port (meilisearch 7700).
	base := buildServiceResponse("meilisearch")
	if !strings.Contains(base.Dashboard, ":7700") {
		t.Fatalf("meilisearch Dashboard with no override = %q, want host port 7700", base.Dashboard)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["meilisearch"] = config.ServiceConfig{Enabled: true, Port: 7700, PublishedPort: 7701}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	moved := buildServiceResponse("meilisearch")
	if !strings.Contains(moved.Dashboard, ":7701") || strings.Contains(moved.Dashboard, ":7700") {
		t.Errorf("meilisearch Dashboard after move = %q, want host port 7701 not 7700", moved.Dashboard)
	}
}

// TestBuildServiceResponse_secondaryPortDashboardUntouched guards the regression
// the other way: mailpit's dashboard is its 8025 web UI, published behind the
// primary 1025 SMTP port. A published-port move shifts only the primary, so the
// dashboard must keep 8025 rather than being dragged onto the primary's port.
func TestBuildServiceResponse_secondaryPortDashboardUntouched(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	base := buildServiceResponse("mailpit")
	if !strings.Contains(base.Dashboard, ":8025") {
		t.Fatalf("mailpit Dashboard = %q, want the 8025 web UI port", base.Dashboard)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["mailpit"] = config.ServiceConfig{Enabled: true, Port: 1025, PublishedPort: 1026}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	moved := buildServiceResponse("mailpit")
	if !strings.Contains(moved.Dashboard, ":8025") || strings.Contains(moved.Dashboard, ":1026") {
		t.Errorf("mailpit Dashboard after primary move = %q, want the untouched 8025 web UI", moved.Dashboard)
	}

	// Now move the 8025 UI mapping itself: the dashboard must follow to 8026.
	cfg.Services["mailpit"] = config.ServiceConfig{Enabled: true, Port: 1025, PublishedPorts: map[int]int{8025: 8026}}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	uiMoved := buildServiceResponse("mailpit")
	if !strings.Contains(uiMoved.Dashboard, ":8026") || strings.Contains(uiMoved.Dashboard, ":8025") {
		t.Errorf("mailpit Dashboard after UI-port move = %q, want 8026", uiMoved.Dashboard)
	}
}

// TestBuildServiceResponse_secondaryPortsExposed proves the response advertises a
// multi-port service's mappings past the primary so the ports modal can render an
// editable field per published port, with the current override reflected.
func TestBuildServiceResponse_secondaryPortsExposed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	base := buildServiceResponse("mailpit")
	var ui *ServicePortMapping
	for i := range base.SecondaryPorts {
		if base.SecondaryPorts[i].Container == 8025 {
			ui = &base.SecondaryPorts[i]
		}
	}
	if ui == nil {
		t.Fatalf("mailpit SecondaryPorts missing the 8025 mapping: %+v", base.SecondaryPorts)
	}
	if ui.Default != 8025 || ui.Published != 0 {
		t.Errorf("8025 mapping = %+v, want default 8025, no override", *ui)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	cfg.Services["mailpit"] = config.ServiceConfig{Enabled: true, Port: 1025, PublishedPorts: map[int]int{8025: 8026}}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	moved := buildServiceResponse("mailpit")
	for _, p := range moved.SecondaryPorts {
		if p.Container == 8025 && p.Published != 8026 {
			t.Errorf("8025 mapping Published = %d, want 8026", p.Published)
		}
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
