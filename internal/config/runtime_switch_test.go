package config

import (
	"os"
	"testing"
	"time"
)

// While a runtime switch is running, containers stop and start and sites answer
// 500 for a few seconds. Every status surface read that as breakage. The marker
// lets them say a switch is in progress instead.
func TestRuntimeSwitchMarker(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if RuntimeSwitchInProgress() {
		t.Fatal("no switch has started")
	}
	if err := MarkRuntimeSwitch(); err != nil {
		t.Fatalf("MarkRuntimeSwitch: %v", err)
	}
	if !RuntimeSwitchInProgress() {
		t.Error("a started switch must read as in progress")
	}
	ClearRuntimeSwitch()
	if RuntimeSwitchInProgress() {
		t.Error("a finished switch must not linger")
	}
}

// A switch killed part-way leaves the marker behind. It must expire, or lerd
// would claim to be switching forever and never report a real failure again.
func TestRuntimeSwitchMarkerExpires(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := MarkRuntimeSwitch(); err != nil {
		t.Fatalf("MarkRuntimeSwitch: %v", err)
	}
	stale := time.Now().Add(-runtimeSwitchTTL - time.Minute)
	if err := os.Chtimes(runtimeSwitchMarkerPath(), stale, stale); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if RuntimeSwitchInProgress() {
		t.Error("a stale marker must not keep claiming a switch")
	}
}
