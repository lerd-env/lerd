package serviceops

import (
	"errors"
	"testing"
)

func stubVhostSync(t *testing.T, changed bool, syncErr error) *int {
	t.Helper()
	reloads := 0
	prevSync, prevReload := syncLerdVhostFn, domainReloadFn
	syncLerdVhostFn = func() (bool, error) { return changed, syncErr }
	domainReloadFn = func() error { reloads++; return nil }
	t.Cleanup(func() { syncLerdVhostFn, domainReloadFn = prevSync, prevReload })
	return &reloads
}

// A vhost that moved is only serving once nginx has read it again.
func TestSyncDashboardVhost_ReloadsWhenChanged(t *testing.T) {
	reloads := stubVhostSync(t, true, nil)
	syncDashboardVhost()
	if *reloads != 1 {
		t.Errorf("reloads = %d, want 1 after the vhost changed", *reloads)
	}
}

// Most service operations move nothing, and reloading nginx on each one would
// drop connections for no reason.
func TestSyncDashboardVhost_NoReloadWhenUnchanged(t *testing.T) {
	reloads := stubVhostSync(t, false, nil)
	syncDashboardVhost()
	if *reloads != 0 {
		t.Errorf("reloads = %d, want none when the vhost did not change", *reloads)
	}
}

// The dashboard is not worth failing an install over, so a sync that fails is
// reported and the operation carries on.
func TestSyncDashboardVhost_SurvivesAFailedSync(t *testing.T) {
	reloads := stubVhostSync(t, false, errors.New("disk full"))
	syncDashboardVhost()
	if *reloads != 0 {
		t.Errorf("reloads = %d, want none when the sync failed", *reloads)
	}
}
