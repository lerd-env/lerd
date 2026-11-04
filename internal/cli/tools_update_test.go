package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubOutdatedTools makes the host look like one where composer and fnm are
// both installed at a version that is not the pinned one, so the update loop
// has more than one tool to work through.
func stubOutdatedTools(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	bin := filepath.Join(home, "lerd", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"composer.phar", "fnm"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, name+".version"), []byte("0.0.1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A tool that cannot be updated must not stop the others. A published digest
// that does not match is a per-tool failure, and before this the first one
// returned early and left every later tool on its old version.
func TestRunToolsUpdateContinuesPastAFailure(t *testing.T) {
	stubOutdatedTools(t)

	var attempted []string
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })
	updateToolFn = func(_ *pinnedTools, name string) error {
		attempted = append(attempted, name)
		if name == "composer" {
			return errors.New("checksum mismatch")
		}
		return nil
	}

	err := runToolsUpdate(nil, nil)

	if len(attempted) < 2 {
		t.Fatalf("only attempted %v; the run stopped at the first failure", attempted)
	}
	if attempted[0] != "composer" {
		t.Fatalf("expected composer first, got %v", attempted)
	}
	if !containsName(attempted, "fnm") {
		t.Errorf("fnm was never attempted after composer failed: %v", attempted)
	}
	if err == nil {
		t.Error("a failed tool must still be reported in the return value")
	}
}

// Every tool succeeding reports no error.
func TestRunToolsUpdateReportsSuccess(t *testing.T) {
	stubOutdatedTools(t)
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })
	updateToolFn = func(*pinnedTools, string) error { return nil }

	if err := runToolsUpdate(nil, nil); err != nil {
		t.Errorf("runToolsUpdate = %v, want nil", err)
	}
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// A tool the user installed is shown but never touched. An unknown version is
// the case that bites: the loop reinstalls lerd's own tools when it sees one.
func TestRunToolsUpdateLeavesTheUsersMiseAlone(t *testing.T) {
	stubOutdatedTools(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(config.ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.GlobalConfigFile(), []byte("node:\n  manager: mise\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mise := filepath.Join(home, ".local", "bin", "mise")
	if err := os.MkdirAll(filepath.Dir(mise), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mise, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var attempted []string
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })
	updateToolFn = func(_ *pinnedTools, name string) error {
		attempted = append(attempted, name)
		return nil
	}

	if err := runToolsUpdate(nil, nil); err != nil {
		t.Fatal(err)
	}
	if containsName(attempted, "mise") {
		t.Errorf("tried to update the user's own mise: %v", attempted)
	}
	if !containsName(attempted, "composer") {
		t.Errorf("lerd's own tools were not updated: %v", attempted)
	}
}
