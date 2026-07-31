package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestLANServicesCommandPersistsExplicitOptIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.LAN.Exposed = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	cmd := newLANServicesCmd()
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("services on: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after on: %v", err)
	}
	if !cfg.LAN.ServicesExposed {
		t.Fatal("services on did not persist lan.services_exposed")
	}

	cmd = newLANServicesCmd()
	cmd.SetArgs([]string{"off"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("services off: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after off: %v", err)
	}
	if cfg.LAN.ServicesExposed {
		t.Fatal("services off did not clear lan.services_exposed")
	}
}

func TestLANServicesCommandRejectsUnknownState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newLANServicesCmd()
	cmd.SetArgs([]string{"maybe"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("services command accepted an unknown state")
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.LAN.ServicesExposed {
		t.Fatal("invalid state changed lan.services_exposed")
	}
}

// Opting in while lerd is loopback only would store a preference that
// publishes nothing, so the command refuses rather than pretending.
func TestLANServicesRefusesToEnableWhileLoopbackOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newLANServicesCmd()
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("services on succeeded while LAN exposure was off")
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.LAN.ServicesExposed {
		t.Fatal("refused command still persisted lan.services_exposed")
	}
}

// Clearing a setting armed before an unexpose must stay possible.
func TestLANServicesStillDisablesWhileLoopbackOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.LAN.ServicesExposed = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	cmd := newLANServicesCmd()
	cmd.SetArgs([]string{"off"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("services off: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.LAN.ServicesExposed {
		t.Fatal("services off did not clear the setting")
	}
}
