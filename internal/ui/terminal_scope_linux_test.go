package ui

import (
	"errors"
	"slices"
	"testing"
)

// lerd update restarts lerd-ui, and systemd stops a unit by killing every
// process in it, so a terminal lerd-ui opens has to live in a scope of its own
// or the update dies at "Starting lerd-ui".
func TestOwnScope_runsTheTerminalInItsOwnSystemdScope(t *testing.T) {
	prev := scopeLookPath
	scopeLookPath = func(string) (string, error) { return "/usr/bin/systemd-run", nil }
	t.Cleanup(func() { scopeLookPath = prev })

	bin, args := ownScope("/usr/bin/konsole", []string{"--separate", "-e", "sh", "-c", "lerd update"})
	if bin != "/usr/bin/systemd-run" {
		t.Fatalf("bin = %q, want systemd-run", bin)
	}
	want := []string{"--user", "--scope", "--quiet", "--collect", "--", "/usr/bin/konsole", "--separate", "-e", "sh", "-c", "lerd update"}
	if !slices.Equal(args, want) {
		t.Errorf("args = %q, want %q", args, want)
	}
}

// Without systemd-run there is no scope to move into, and the terminal still opens.
func TestOwnScope_leavesTheCommandAloneWithoutSystemdRun(t *testing.T) {
	prev := scopeLookPath
	scopeLookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { scopeLookPath = prev })

	bin, args := ownScope("/usr/bin/xterm", []string{"-e", "sh"})
	if bin != "/usr/bin/xterm" || !slices.Equal(args, []string{"-e", "sh"}) {
		t.Errorf("got %q %q, want the command unchanged", bin, args)
	}
}
