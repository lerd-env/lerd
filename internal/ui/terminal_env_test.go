package ui

import (
	"os"
	"strings"
	"testing"
)

func TestGraphicalEnvPreservesBaseEnvAndPatchesDisplay(t *testing.T) {
	t.Setenv("LERD_TEST_SENTINEL", "abc123")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	if err := os.WriteFile(runtimeDir+"/wayland-7", []byte{}, 0o600); err != nil {
		t.Fatalf("seed wayland socket: %v", err)
	}

	env := graphicalEnv()

	var sawSentinel, sawWayland, sawRuntimeDir bool
	var waylandVal string
	for _, kv := range env {
		switch {
		case kv == "LERD_TEST_SENTINEL=abc123":
			sawSentinel = true
		case strings.HasPrefix(kv, "WAYLAND_DISPLAY="):
			sawWayland = true
			waylandVal = strings.TrimPrefix(kv, "WAYLAND_DISPLAY=")
		case kv == "XDG_RUNTIME_DIR="+runtimeDir:
			sawRuntimeDir = true
		}
	}

	if !sawSentinel {
		t.Error("graphicalEnv dropped base environment entry")
	}
	if !sawRuntimeDir {
		t.Error("graphicalEnv did not preserve XDG_RUNTIME_DIR")
	}
	if !sawWayland {
		t.Error("graphicalEnv did not probe WAYLAND_DISPLAY from XDG_RUNTIME_DIR")
	}
	if sawWayland && waylandVal != "wayland-7" {
		t.Errorf("WAYLAND_DISPLAY = %q, want wayland-7", waylandVal)
	}
}

func TestTerminalDirCandidatesOpenPtyxisInDir(t *testing.T) {
	t.Setenv("TERMINAL", "")
	const dir = "/home/user/project"
	var ptyxis *terminalCmd
	for _, c := range terminalDirCandidates(dir) {
		if c.bin == "ptyxis" {
			cp := c
			ptyxis = &cp
		}
	}
	if ptyxis == nil {
		t.Fatal("ptyxis is not among the terminal candidates")
	}
	joined := strings.Join(ptyxis.args, " ")
	// ptyxis is single-instance: without --new-window (or --tab/-x) it ignores
	// --working-directory and opens a new window in $HOME instead of the site.
	if !strings.Contains(joined, "--new-window") {
		t.Errorf("ptyxis args %v lack --new-window, so %q would be ignored", ptyxis.args, dir)
	}
	if !strings.Contains(joined, dir) {
		t.Errorf("ptyxis args %v do not carry the target dir %q", ptyxis.args, dir)
	}
}

func TestGraphicalEnvDoesNotDuplicateKeys(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	env := graphicalEnv()
	count := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "XDG_SESSION_TYPE=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("XDG_SESSION_TYPE appears %d times, want 1", count)
	}
}
