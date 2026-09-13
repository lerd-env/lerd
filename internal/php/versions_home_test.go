package php

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The launchd unit dir follows HOME rather than the XDG vars, so a test that
// stages only XDG still reads whatever PHP versions the developer's machine has
// installed. ListInstalled must answer from the staged home.
func TestListInstalled_ReadsTheStagedHomeNotTheMachine(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd units are macOS only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "lerd-php81-fpm.plist"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := listInstalledFromServiceDir()
	if len(got) != 1 || got[0] != "8.1" {
		t.Errorf("got %v, want only the staged [8.1]", got)
	}
}
