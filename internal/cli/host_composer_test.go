package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/feedback"
)

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestHostComposerOnPathIgnoresLerdsOwnShim(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	binDir := filepath.Join(tmp, "lerd", "bin")
	writeExecutable(t, filepath.Join(binDir, "composer"))
	t.Setenv("PATH", binDir)

	if got := hostComposerOnPath(); got != "" {
		t.Errorf("lerd's own shim must not count as a host composer, got %q", got)
	}
}

func TestHostComposerOnPathFindsTheOneBehindTheShim(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	binDir := filepath.Join(tmp, "lerd", "bin")
	writeExecutable(t, filepath.Join(binDir, "composer"))
	hostDir := filepath.Join(tmp, "usr", "bin")
	host := filepath.Join(hostDir, "composer")
	writeExecutable(t, host)
	t.Setenv("PATH", strings.Join([]string{binDir, hostDir}, string(os.PathListSeparator)))

	if got := hostComposerOnPath(); got != host {
		t.Errorf("got %q, want %q", got, host)
	}
}

func TestHostComposerOnPathSkipsNonExecutable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := filepath.Join(tmp, "share")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer"), []byte("notes"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if got := hostComposerOnPath(); got != "" {
		t.Errorf("a non-executable file is not a composer, got %q", got)
	}
}

func TestNoteShadowedComposerNamesTheHostCopy(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	hostDir := filepath.Join(tmp, "usr", "bin")
	host := filepath.Join(hostDir, "composer")
	writeExecutable(t, host)
	t.Setenv("PATH", hostDir)

	var buf bytes.Buffer
	restore := feedback.SetTestWriter(&buf)
	defer restore()

	noteShadowedComposer()

	if !strings.Contains(buf.String(), host) {
		t.Errorf("install should name the composer it fronts, got %q", buf.String())
	}
}

func TestNoteShadowedComposerSilentWithoutOne(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("PATH", filepath.Join(tmp, "lerd", "bin"))

	var buf bytes.Buffer
	restore := feedback.SetTestWriter(&buf)
	defer restore()

	noteShadowedComposer()

	if buf.Len() != 0 {
		t.Errorf("nothing to say when the user has no composer of their own, got %q", buf.String())
	}
}
