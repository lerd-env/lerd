package node

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireNvm skips the calling test unless a usable nvm with a default version
// is installed, so CI (and any host without nvm) stays green. Returns the
// backend for the test to drive.
func requireNvm(t *testing.T) nvmManager {
	t.Helper()
	m := nvmManager{}
	if !m.Available() || !m.HasDefault() {
		t.Skip("nvm with a default version not available; skipping integration test")
	}
	return m
}

// wantNodeVersion runs cmd and fails unless it prints a v-prefixed Node version.
func wantNodeVersion(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "v") {
		t.Fatalf("output = %q, want a v-prefixed node version", strings.TrimSpace(string(out)))
	}
}

// TestNvmCommand_RunsNode drives the Command path used by the CLI/UI/MCP: it
// sources the user's nvm.sh, selects the default version, and runs node.
func TestNvmCommand_RunsNode(t *testing.T) {
	m := requireNvm(t)
	wantNodeVersion(t, m.Command("default", "node", []string{"--version"}))
}

// TestNvmExecPrefix_SurvivesGuardLine reproduces the macOS host-worker guard
// line, where ExecPrefix is placed raw into a shell-parsed line followed by the
// command: `exec <prefix> /bin/sh -c '<cmd>'`. This guards the deeply nested
// quoting (a shellQuote'd bash -c script inside a shell-parsed line) that the
// worker builders rely on. The Linux path embeds the same prefix in a systemd
// ExecStart, which applies the same shell-style quote parsing.
func TestNvmExecPrefix_SurvivesGuardLine(t *testing.T) {
	m := requireNvm(t)
	guardLine := "exec " + m.ExecPrefix("default") + " /bin/sh -c 'node --version'"
	wantNodeVersion(t, exec.Command("sh", "-c", guardLine))
}

// TestNvmShimScript_FallsBackToNvm writes the generated node shim to disk and
// runs it with an unreachable lerd binary, so it exercises the direct-nvm
// fallback branch (bash sourcing nvm.sh, selecting the default, exec'ing node).
func TestNvmShimScript_FallsBackToNvm(t *testing.T) {
	m := requireNvm(t)
	dir := t.TempDir()
	shimPath := filepath.Join(dir, "node")
	// A lerd path that does not exist forces the shim past its lerd branch.
	shim := m.ShimScript(filepath.Join(dir, "does-not-exist-lerd"), "node")
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shimPath, "--version")
	cmd.Dir = dir // no .node-version here, so the shim uses the nvm default
	wantNodeVersion(t, cmd)
}
