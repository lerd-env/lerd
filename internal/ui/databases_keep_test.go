package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// seedUISnapshot writes a snapshot sidecar the way serviceops stores one.
func seedUISnapshot(t *testing.T, snap serviceops.Snapshot) {
	t.Helper()
	dir := filepath.Join(config.SnapshotsDir(), snap.Service, "databases", snap.Database, snap.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

func TestHandleSnapshotKeep(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	seedUISnapshot(t, serviceops.Snapshot{
		Name: "auto-1", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "shop_db", Auto: true,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/mysql/snapshot/keep", strings.NewReader(`{"database":"shop_db","name":"auto-1","kept":true}`))
	handleSnapshotKeep(rec, req, "mysql")
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("keep failed: %s", rec.Body.String())
	}
	snaps, err := serviceops.ListSnapshots("mysql", "shop_db", false)
	if err != nil || len(snaps) != 1 || !snaps[0].Kept {
		t.Fatalf("snapshot not kept: %+v (%v)", snaps, err)
	}

	// A manual snapshot is already permanent, and says so rather than pretending.
	seedUISnapshot(t, serviceops.Snapshot{
		Name: "by-hand", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "shop_db",
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/databases/mysql/snapshot/keep", strings.NewReader(`{"database":"shop_db","name":"by-hand","kept":true}`))
	handleSnapshotKeep(rec, req, "mysql")
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("keeping a manual snapshot should be refused, got %s", rec.Body.String())
	}
}

// The list carries the expiry the current policy implies, so the browser never
// has to know the retention rules.
func TestWithRetention_carriesExpiry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := os.MkdirAll(config.ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.GlobalConfigFile(), []byte("auto_snapshot:\n  enabled: true\n  every: 24h\n  keep: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	rows := withRetention([]serviceops.Snapshot{
		{Name: "auto-1", Created: now, Service: "mysql", Database: "shop_db", Auto: true},
		{Name: "manual", Created: now.Add(-time.Hour), Service: "mysql", Database: "shop_db"},
	})
	if rows[0].ExpiresAt == nil || rows[0].RunsLeft != 2 || !rows[0].Estimated {
		t.Errorf("automatic snapshot = %+v, want a two-run estimate", rows[0])
	}
	if rows[1].ExpiresAt != nil || rows[1].RunsLeft != 0 {
		t.Errorf("manual snapshot = %+v, want no expiry", rows[1])
	}
}
