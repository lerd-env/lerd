package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// runHostAndReap must end the orphan before propagating: the exit path calls
// os.Exit, which skips defers, so a deferred reap never ran and the Playwright
// server pest-plugin-browser started outlived every `lerd test`.
func TestRunHostAndReapKillsTheOrphanBeforeReturning(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("/bin/sh", "-c", "sleep 30 & echo $! > "+pidFile+"; exit 0")

	if err := runHostAndReap(cmd); err != nil {
		t.Fatalf("run: %v", err)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if syscall.Kill(child, 0) == nil {
		_ = syscall.Kill(child, syscall.SIGKILL)
		t.Error("the orphaned child survived the run")
	}
}
