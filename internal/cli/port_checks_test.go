package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestCollectPortChecks_usesPublishedPortOverride covers the PublishedPort > 0
// branch: when a built-in service has been moved off its preset default port
// (e.g. lerd-mysql 3306 → 3307 to free 3306 for a host server), the boot-time
// port conflict check must verify the REAL bound port, not the preset default.
func TestCollectPortChecks_usesPublishedPortOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Pick a real known DB service so knownServices() enumerates it.
	svc := "mysql"
	if !isKnownService(svc) {
		t.Skipf("%q is not a default preset on this build", svc)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.Services[svc] = config.ServiceConfig{Enabled: true, Port: 3306, PublishedPort: 3307}
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	checks := CollectPortChecks([]string{"lerd-" + svc})
	var sawPublished, sawDefault bool
	for _, c := range checks {
		if c.Container != "lerd-"+svc {
			continue
		}
		switch c.Port {
		case "3307":
			sawPublished = true
		case "3306":
			sawDefault = true
		}
	}
	if !sawPublished {
		t.Errorf("CollectPortChecks must check the overridden published port 3307; got %+v", checks)
	}
	if sawDefault {
		t.Errorf("CollectPortChecks must NOT check the vacated default port 3306 when overridden; got %+v", checks)
	}
}
