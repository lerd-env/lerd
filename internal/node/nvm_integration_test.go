package node

import (
	"os/exec"
	"strings"
	"testing"
)

// requireNvm skips the calling test unless nvm is installed and activating the
// default version actually yields a Node binary. HasDefault alone is not enough:
// `nvm version default` returns "system" when the alias points at the host node,
// which would leave CI red on runners whose default is system.
func requireNvm(t *testing.T) nvmManager {
	t.Helper()
	m := nvmManager{}
	if !m.Available() {
		t.Skip("nvm not available; skipping integration test")
	}
	out, err := m.Command("default", "node", []string{"--version"}).CombinedOutput()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(out)), "v") {
		t.Skipf("nvm available but activation yields no Node (%v: %s); skipping", err, strings.TrimSpace(string(out)))
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
