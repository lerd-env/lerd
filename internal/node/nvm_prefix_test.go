package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeNvm writes an nvm.sh whose `nvm use` always refuses, the way real nvm
// does when ~/.npmrc sets a prefix, and whose `nvm which` prints $FAKE_WHICH.
func fakeNvm(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	t.Setenv("NVM_DIR", dir)
	script := `nvm() { case "$1" in which) [ -n "$FAKE_WHICH" ] && echo "$FAKE_WHICH" ;; use) return 11 ;; esac; }` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "nvm.sh"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "versions", "node", "v22.9.0", "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho fake-node-22\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(bin, "node")
}

// An npm prefix in ~/.npmrc makes `nvm use` refuse, which used to leave every
// npm step of a site's setup failing on a machine that has the version installed.
func TestNvmActivationSurvivesAnNpmPrefix(t *testing.T) {
	node := fakeNvm(t)
	t.Setenv("FAKE_WHICH", node)

	out, err := (nvmManager{}).Command("22", "node", nil).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "fake-node-22" {
		t.Fatalf("got %q, %v; want the nvm node to run", out, err)
	}
}

// A default alias pointing at the host node resolves outside $NVM_DIR, and that
// is not an nvm-managed Node, so activation still refuses it.
func TestNvmActivationRefusesTheSystemNode(t *testing.T) {
	fakeNvm(t)
	t.Setenv("FAKE_WHICH", "/usr/bin/node")

	out, err := (nvmManager{}).Command("22", "node", nil).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "no nvm Node available for 22") {
		t.Fatalf("got %q, %v; want a refusal", out, err)
	}
}
