package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMissingBinaryFallback_PrefersSiblingOfRunningBinary(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	name := "lerd-sibling-probe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	sibling := filepath.Join(filepath.Dir(self), name)
	if err := os.WriteFile(sibling, []byte("x"), 0o755); err != nil {
		t.Skipf("cannot write beside test binary: %v", err)
	}
	t.Cleanup(func() { os.Remove(sibling) })

	missing := filepath.Join(t.TempDir(), "lerd-sibling-probe")
	if got := missingBinaryFallback(missing); got != sibling {
		t.Fatalf("got %q, want sibling %q", got, sibling)
	}
}

func TestMissingBinaryFallback_FallsBackToSelf(t *testing.T) {
	self, _ := os.Executable()
	missing := filepath.Join(t.TempDir(), "no-such-helper")
	if got := missingBinaryFallback(missing); got != self {
		t.Fatalf("got %q, want self %q", got, self)
	}
}
