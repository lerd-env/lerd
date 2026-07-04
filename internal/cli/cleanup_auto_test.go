package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/cleanup"
	"github.com/geodro/lerd/internal/config"
)

// The held-by-containers hint is silent when nothing is held, and otherwise
// names the size and points at restart as the way to release it.
func TestHeldHint(t *testing.T) {
	if got := heldHint(cleanup.Plan{}); got != "" {
		t.Errorf("no held images should give an empty hint, got %q", got)
	}
	got := heldHint(cleanup.Plan{Held: cleanup.HeldByContainers{Count: 8, Bytes: 2 << 30}})
	if !strings.Contains(got, "8 image") || !strings.Contains(got, "lerd restart") {
		t.Errorf("hint should mention the count and restart, got %q", got)
	}
}

// cleanup runs the deep tier by default; --safe opts back down to the
// conservative sweep, and the old --deep flag stays as a hidden no-op.
func TestNewCleanupCmd_DeepByDefaultSafeOptOut(t *testing.T) {
	cmd := NewCleanupCmd()
	safe := cmd.Flags().Lookup("safe")
	if safe == nil {
		t.Fatal("expected a --safe opt-out flag")
	}
	if safe.DefValue != "false" {
		t.Errorf("--safe should default false so deep is on by default, got %q", safe.DefValue)
	}
	deep := cmd.Flags().Lookup("deep")
	if deep == nil || !deep.Hidden {
		t.Error("--deep should remain as a hidden deprecated no-op")
	}
}

// runCleanupAutoToggle persists the auto_cleanup flag so the watcher and event
// hooks read it back. Default is on; off then on must round-trip through config.
func TestCleanupAutoToggle_PersistsFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoCleanupEnabled() {
		t.Fatal("auto cleanup should start enabled by default")
	}

	if err := runCleanupAutoToggle(false); err != nil {
		t.Fatalf("toggle off: %v", err)
	}
	if cfg, _ := config.LoadGlobal(); cfg.AutoCleanupEnabled() {
		t.Error("expected disabled after toggle off")
	}

	if err := runCleanupAutoToggle(true); err != nil {
		t.Fatalf("toggle on: %v", err)
	}
	if cfg, _ := config.LoadGlobal(); !cfg.AutoCleanupEnabled() {
		t.Error("expected enabled after toggle on")
	}
}
