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

// A stop that lands before podman starts a container leaves it in Created,
// where --rm never fires, so Stop has to remove containers that exist but do
// not run, or a stripe listener outlives its unit.
func TestStop_RemovesContainerThatNeverStarted(t *testing.T) {
	prevExists, prevRemove, prevLaunchctl := containerExistsFn, removeContainerFn, launchctl
	t.Cleanup(func() { containerExistsFn, removeContainerFn, launchctl = prevExists, prevRemove, prevLaunchctl })

	containerExistsFn = func(string) (bool, error) { return true, nil }
	var removed []string
	removeContainerFn = func(name string) { removed = append(removed, name) }
	launchctl = func(...string) ([]byte, error) { return nil, nil }

	if err := (&darwinServiceManager{}).Stop("lerd-stripe-app"); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "lerd-stripe-app" {
		t.Errorf("removed %v, want the created container removed", removed)
	}
}

// A sweep used to fork `launchctl print` once per plist, which the dashboard's
// five second health watch turned into dozens of forks a tick. One list has to
// answer every unit it can.
func TestAllUnitStates_ReadsOneListInsteadOfPrintPerUnit(t *testing.T) {
	dir := t.TempDir()
	plists := map[string]string{
		"lerd-queue-a":    "<plist/>",
		"lerd-queue-b":    "<plist/>",
		"lerd-schedule-c": "<plist/>",
		"lerd-queue-gone": "<plist/>",
		"lerd-mysql":      "<string>/opt/homebrew/bin/podman</string><string>run</string>",
	}
	for name, body := range plists {
		if err := os.WriteFile(filepath.Join(dir, name+".plist"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	prevDir, prevSnap, prevLaunchctl := launchAgentsDirFn, containerSnapshotFn, launchctl
	t.Cleanup(func() { launchAgentsDirFn, containerSnapshotFn, launchctl = prevDir, prevSnap, prevLaunchctl })
	launchAgentsDirFn = func() string { return dir }
	containerSnapshotFn = func() map[string]bool { return map[string]bool{"lerd-mysql": true} }

	prints := 0
	launchctl = func(args ...string) ([]byte, error) {
		if args[0] == "list" {
			return []byte("PID\tStatus\tLabel\n" +
				"412\t0\tcom.lerd.lerd-queue-a\n" +
				"-\t1\tcom.lerd.lerd-queue-b\n" +
				"-\t0\tcom.lerd.lerd-schedule-c\n" +
				"-\t0\tcom.lerd.lerd-mysql\n"), nil
		}
		prints++
		return nil, os.ErrNotExist
	}

	got := (&darwinServiceManager{}).AllUnitStates()
	want := map[string]string{
		"lerd-queue-a": "active", "lerd-queue-b": "failed", "lerd-schedule-c": "inactive",
		"lerd-queue-gone": "inactive", "lerd-mysql": "active",
	}
	for name, state := range want {
		if got[name] != state {
			t.Errorf("%s = %q, want %q", name, got[name], state)
		}
	}
	if prints != 0 {
		t.Errorf("launchctl print ran %d times, want 0 when the list answers every unit", prints)
	}
}

// A container launcher that exited 0 with its container down is either crashed
// or never ran, and only launchctl print tells those apart.
func TestListedStatus_ContainerExitZeroDefersToPrint(t *testing.T) {
	plist := filepath.Join(t.TempDir(), "lerd-mysql.plist")
	if err := os.WriteFile(plist, []byte("<string>/opt/homebrew/bin/podman</string><string>run</string>"), 0644); err != nil {
		t.Fatal(err)
	}
	jobs := parseLaunchctlList("-\t0\tcom.lerd.lerd-mysql\n")
	if _, ok := listedStatus("lerd-mysql", plist, jobs, map[string]bool{}); ok {
		t.Error("a stopped container launcher with exit 0 must defer to launchctl print")
	}
	if _, ok := listedStatus("lerd-mysql", plist, nil, map[string]bool{}); ok {
		t.Error("without a list every unit must defer to launchctl print")
	}
}
