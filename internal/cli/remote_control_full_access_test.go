package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// withCredentials writes a config carrying a dashboard password so the
// full-access command has something to widen.
func withCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	cfg.UI.Username = "alice"
	cfg.UI.PasswordHash = "$2a$04$abcdefghijklmnopqrstuv"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

func TestFullAccessCommandPersistsExplicitOptIn(t *testing.T) {
	withCredentials(t)

	cmd := newRemoteControlFullAccessCmd()
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("full-access on: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after on: %v", err)
	}
	if !cfg.UI.RemoteFullAccess {
		t.Fatal("full-access on did not persist ui.remote_full_access")
	}

	cmd = newRemoteControlFullAccessCmd()
	cmd.SetArgs([]string{"off"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("full-access off: %v", err)
	}
	cfg, err = config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal after off: %v", err)
	}
	if cfg.UI.RemoteFullAccess {
		t.Fatal("full-access off did not clear ui.remote_full_access")
	}
}

func TestFullAccessCommandRequiresCredentials(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := newRemoteControlFullAccessCmd()
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("full-access on succeeded without dashboard credentials")
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.UI.RemoteFullAccess {
		t.Fatal("failed full-access on still changed ui.remote_full_access")
	}
}

func TestFullAccessCommandRejectsUnknownState(t *testing.T) {
	withCredentials(t)

	cmd := newRemoteControlFullAccessCmd()
	cmd.SetArgs([]string{"maybe"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("full-access accepted an unknown state")
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.UI.RemoteFullAccess {
		t.Fatal("invalid state changed ui.remote_full_access")
	}
}

// Turning remote access off entirely must not leave the widened authority
// behind, waiting to apply to whatever password is set next.
func TestRemoteControlOffClearsFullAccess(t *testing.T) {
	withCredentials(t)

	cmd := newRemoteControlFullAccessCmd()
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("full-access on: %v", err)
	}

	off := newRemoteControlOffCmd()
	off.SetArgs(nil)
	if err := off.Execute(); err != nil {
		t.Fatalf("remote-control off: %v", err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.UI.RemoteFullAccess {
		t.Fatal("remote-control off left ui.remote_full_access set")
	}
}
