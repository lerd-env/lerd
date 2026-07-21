//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"testing"
)

// AllUnitStates used to resolve container liveness one unit at a time. In any
// process without the container cache running (every CLI invocation, and the
// MCP server) each of those is a `podman inspect` subprocess and a round trip
// into the podman VM, so the sweep cost scaled with the number of workers.
// One query has to serve the whole sweep no matter how many plists there are.
func TestAllUnitStates_QueriesContainersOncePerSweep(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"lerd-queue-a", "lerd-queue-b", "lerd-horizon-c", "lerd-schedule-d"} {
		if err := os.WriteFile(filepath.Join(dir, name+".plist"), []byte("<plist/>"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	prevDir := launchAgentsDirFn
	launchAgentsDirFn = func() string { return dir }
	t.Cleanup(func() { launchAgentsDirFn = prevDir })

	calls := 0
	prevSnap := containerSnapshotFn
	containerSnapshotFn = func() map[string]bool {
		calls++
		return map[string]bool{}
	}
	t.Cleanup(func() { containerSnapshotFn = prevSnap })

	m := &darwinServiceManager{}
	m.AllUnitStates()

	if calls != 1 {
		t.Errorf("container state queried %d times for a 4 unit sweep, want exactly 1", calls)
	}
}

// The snapshot is only a batching shortcut, so it must give the same answer the
// per-unit lookup would: a unit whose container is up reads as running.
func TestContainerRunning_UsesSnapshotWhenPresent(t *testing.T) {
	snap := map[string]bool{"lerd-queue-a": true, "lerd-queue-b": false}

	if !containerRunning("lerd-queue-a", snap) {
		t.Error("unit present and running in the snapshot must read as running")
	}
	if containerRunning("lerd-queue-b", snap) {
		t.Error("unit present but stopped in the snapshot must read as not running")
	}
	if containerRunning("lerd-missing", snap) {
		t.Error("unit absent from the snapshot must read as not running")
	}
}
