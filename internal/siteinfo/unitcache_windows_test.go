//go:build windows

package siteinfo

import (
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

type fakeWinLifecycle struct {
	states map[string]string
	sweeps int
}

func (f *fakeWinLifecycle) Start(string) error                { return nil }
func (f *fakeWinLifecycle) Stop(string) error                 { return nil }
func (f *fakeWinLifecycle) Restart(string) error              { return nil }
func (f *fakeWinLifecycle) UnitStatus(string) (string, error) { return "unknown", nil }
func (f *fakeWinLifecycle) AllUnitStates() map[string]string  { f.sweeps++; return f.states }

func useFakeWinLifecycle(t *testing.T, states map[string]string) *fakeWinLifecycle {
	t.Helper()
	f := &fakeWinLifecycle{states: states}
	prev := podman.UnitLifecycle
	podman.UnitLifecycle = f
	InvalidateUnitCache()
	t.Cleanup(func() {
		podman.UnitLifecycle = prev
		InvalidateUnitCache()
	})
	return f
}

// The dashboard reads a worker's state through unitStatusFn. Windows has no
// systemctl to list units, so the state has to come from lerd's own service
// manager, or every running worker shows as stopped.
func TestUnitStatus_windowsReadsTheServiceManager(t *testing.T) {
	f := useFakeWinLifecycle(t, map[string]string{
		"lerd-queue-shop":         "active",
		"lerd-queue-shop.service": "active",
		"lerd-horizon-shop":       "failed",
	})

	for name, want := range map[string]string{
		"lerd-queue-shop":         "active",
		"lerd-queue-shop.service": "active",
		"lerd-horizon-shop":       "failed",
		"lerd-schedule-shop":      "unknown",
	} {
		if got, _ := unitStatusFn(name); got != want {
			t.Errorf("unitStatusFn(%q) = %q, want %q", name, got, want)
		}
	}
	if f.sweeps != 1 {
		t.Errorf("service manager swept %d times for one render, want 1 (cached)", f.sweeps)
	}

	InvalidateUnitCache()
	f.states = map[string]string{"lerd-queue-shop": "inactive"}
	if got, _ := unitStatusFn("lerd-queue-shop"); got != "inactive" {
		t.Errorf("after invalidation unitStatusFn = %q, want the fresh inactive", got)
	}
}
