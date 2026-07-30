package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestLANServicesCommandPersistsExplicitOptIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newLANServicesCmd()
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("services on: %v", err)
	}
	cfg, err := config.LoadGlobal()
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
