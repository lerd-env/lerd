package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/cleanup"
)

func stubDisk(t *testing.T, inspect func() (cleanup.Plan, error), apply func(cleanup.Plan) (int, int64)) {
	t.Helper()
	origInspect, origApply := inspectDisk, applyDisk
	inspectDisk, applyDisk = inspect, apply
	invalidateDiskCache()
	t.Cleanup(func() {
		inspectDisk, applyDisk = origInspect, origApply
		invalidateDiskCache()
	})
}

func TestScanDiskReportsPlan(t *testing.T) {
	plan := cleanup.Plan{
		Targets: []cleanup.Target{
			{Kind: "image", ID: "aaa", Desc: "orphaned build", Bytes: 100},
			{Kind: "image", ID: "bbb", Desc: "dangling image", Bytes: 250},
		},
		Held: cleanup.HeldByContainers{Count: 1, Bytes: 40},
	}
	stubDisk(t, func() (cleanup.Plan, error) { return plan, nil }, nil)

	snap := scanDisk()
	if !snap.Available {
		t.Fatal("Available: got false want true")
	}
	if snap.ReclaimableBytes != 350 {
		t.Errorf("ReclaimableBytes: got %d want 350", snap.ReclaimableBytes)
	}
	if len(snap.Images) != 2 || snap.Images[1].Desc != "dangling image" {
		t.Errorf("Images: got %+v", snap.Images)
	}
	if snap.HeldBytes != 40 || snap.HeldCount != 1 {
		t.Errorf("held: got %d/%d want 40/1", snap.HeldBytes, snap.HeldCount)
	}
}

func TestScanDiskUnavailableOnError(t *testing.T) {
	stubDisk(t, func() (cleanup.Plan, error) { return cleanup.Plan{}, errors.New("podman down") }, nil)
	snap := scanDisk()
	if snap.Available {
		t.Error("a scan error must report Available=false, not an empty reclaim")
	}
}

// The apply path must re-inspect server-side and reclaim that fresh plan rather
// than trusting the client, so an image that became live since the modal opened
// is never posted for removal.
func TestDiskCleanupReinspects(t *testing.T) {
	fresh := cleanup.Plan{Targets: []cleanup.Target{{Kind: "image", ID: "live-now", Bytes: 500}}}
	var applied cleanup.Plan
	stubDisk(t,
		func() (cleanup.Plan, error) { return fresh, nil },
		func(p cleanup.Plan) (int, int64) { applied = p; return len(p.Targets), 500 },
	)

	req := httptest.NewRequest(http.MethodPost, "/api/disk", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	handleDisk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	if len(applied.Targets) != 1 || applied.Targets[0].ID != "live-now" {
		t.Errorf("apply got the client list, not the server re-inspection: %+v", applied.Targets)
	}
	var resp struct {
		OK             bool  `json:"ok"`
		Removed        int   `json:"removed"`
		ReclaimedBytes int64 `json:"reclaimed_bytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Removed != 1 || resp.ReclaimedBytes != 500 {
		t.Errorf("resp: got %+v", resp)
	}
}

func TestDiskCleanupRejectsNonLoopback(t *testing.T) {
	called := false
	stubDisk(t,
		func() (cleanup.Plan, error) { called = true; return cleanup.Plan{}, nil },
		func(p cleanup.Plan) (int, int64) { called = true; return 0, 0 },
	)

	req := httptest.NewRequest(http.MethodPost, "/api/disk", nil)
	req.RemoteAddr = "192.168.1.50:5555"
	rec := httptest.NewRecorder()
	handleDisk(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status: got %d want 403", rec.Code)
	}
	if called {
		t.Error("a non-loopback POST must never reach inspect/apply")
	}
}

func TestDiskGetServesPreview(t *testing.T) {
	stubDisk(t, func() (cleanup.Plan, error) {
		return cleanup.Plan{Targets: []cleanup.Target{{ID: "x", Bytes: 7}}}, nil
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/disk", nil)
	rec := httptest.NewRecorder()
	handleDisk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	var snap diskSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if !snap.Available || snap.ReclaimableBytes != 7 {
		t.Errorf("snapshot: got %+v", snap)
	}
}
