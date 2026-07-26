//go:build linux

package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// fakeLifecycle satisfies podman.UnitLifecycle so StartUnit does not touch the
// real systemd manager during the test.
type fakeLifecycle struct{}

func (fakeLifecycle) Start(string) error                { return nil }
func (fakeLifecycle) Stop(string) error                 { return nil }
func (fakeLifecycle) Restart(string) error              { return nil }
func (fakeLifecycle) UnitStatus(string) (string, error) { return "active", nil }
func (fakeLifecycle) AllUnitStates() map[string]string  { return nil }

// trackingLifecycle records which lifecycle method was called and what status
// to report, so a test can assert restart-vs-start behaviour.
type trackingLifecycle struct {
	starts   []string
	restarts []string
	status   string
}

func (t *trackingLifecycle) Start(name string) error { t.starts = append(t.starts, name); return nil }
func (t *trackingLifecycle) Stop(string) error       { return nil }
func (t *trackingLifecycle) Restart(name string) error {
	t.restarts = append(t.restarts, name)
	return nil
}
func (t *trackingLifecycle) UnitStatus(string) (string, error) { return t.status, nil }
func (t *trackingLifecycle) AllUnitStates() map[string]string  { return nil }

func stubStartAndReload(t *testing.T) *int {
	t.Helper()
	prevLC := podman.UnitLifecycle
	podman.UnitLifecycle = fakeLifecycle{}
	t.Cleanup(func() { podman.UnitLifecycle = prevLC })

	reloads := 0
	prevReload := podman.DaemonReloadFn
	podman.DaemonReloadFn = func() error { reloads++; return nil }
	t.Cleanup(func() { podman.DaemonReloadFn = prevReload })
	return &reloads
}

// A worker unit whose content changed (e.g. a runtime switch re-pointed it at a
// different container) must trigger a daemon-reload before enable/start, or
// systemd keeps acting on the stale cached unit.
func TestWorkerStartForSite_reloadsWhenUnitChanged(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	mgr := &captureWriteMgr{writeChange: true}
	swapServiceMgr(t, mgr)
	reloads := stubStartAndReload(t)

	w := config.FrameworkWorker{Command: "npm run dev", Host: true, Label: "Vite"}
	if err := WorkerStartForSite("ws", "/p/ws", "8.4", "vite", w, false); err != nil {
		t.Fatalf("WorkerStartForSite: %v", err)
	}

	if *reloads == 0 {
		t.Error("expected a daemon-reload after the changed unit, got none")
	}
	if len(mgr.enabled) == 0 {
		t.Error("expected the unit to be enabled after reload")
	}
}

// An unchanged unit must not reload: nothing on disk moved, so the cached unit
// is already correct and a reload would be wasted churn.
func TestWorkerStartForSite_noReloadWhenUnchanged(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	mgr := &captureWriteMgr{writeChange: false}
	swapServiceMgr(t, mgr)
	reloads := stubStartAndReload(t)

	w := config.FrameworkWorker{Command: "npm run dev", Host: true, Label: "Vite"}
	if err := WorkerStartForSite("ws", "/p/ws", "8.4", "vite", w, false); err != nil {
		t.Fatalf("WorkerStartForSite: %v", err)
	}

	if *reloads != 0 {
		t.Errorf("expected no daemon-reload for an unchanged unit, got %d", *reloads)
	}
}

// A worker whose unit changed and is already active must be restarted, not
// started: a start is a no-op on an active unit, so the old process would keep
// running while every status surface reports the new one. Issue #1150.
func TestWorkerStartForSite_restartsWhenUnitChangedAndActive(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	mgr := &captureWriteMgr{writeChange: true}
	swapServiceMgr(t, mgr)

	lc := &trackingLifecycle{status: "active"}
	prevLC := podman.UnitLifecycle
	podman.UnitLifecycle = lc
	t.Cleanup(func() { podman.UnitLifecycle = prevLC })
	podman.DaemonReloadFn = func() error { return nil }

	w := config.FrameworkWorker{Command: "npm run dev", Host: true, Label: "Vite"}
	if err := WorkerStartForSite("ws", "/p/ws", "8.4", "vite", w, false); err != nil {
		t.Fatalf("WorkerStartForSite: %v", err)
	}

	if len(lc.restarts) == 0 {
		t.Error("expected a restart when the unit changed and is active, got none")
	}
	if len(lc.starts) != 0 {
		t.Errorf("expected no start call when restarting, got %d", len(lc.starts))
	}
}

// A worker whose unit changed but is NOT active yet should start, not restart:
// there is nothing to restart, and a start is the correct first-run action.
func TestWorkerStartForSite_startsWhenUnitChangedButInactive(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	mgr := &captureWriteMgr{writeChange: true}
	swapServiceMgr(t, mgr)

	lc := &trackingLifecycle{status: "inactive"}
	prevLC := podman.UnitLifecycle
	podman.UnitLifecycle = lc
	t.Cleanup(func() { podman.UnitLifecycle = prevLC })
	podman.DaemonReloadFn = func() error { return nil }

	w := config.FrameworkWorker{Command: "npm run dev", Host: true, Label: "Vite"}
	if err := WorkerStartForSite("ws", "/p/ws", "8.4", "vite", w, false); err != nil {
		t.Fatalf("WorkerStartForSite: %v", err)
	}

	if len(lc.starts) == 0 {
		t.Error("expected a start when the unit changed but is inactive, got none")
	}
	if len(lc.restarts) != 0 {
		t.Errorf("expected no restart when inactive, got %d", len(lc.restarts))
	}
}
