package serviceops

import (
	"os"
	"testing"
	"time"
)

// A snapshot created as "before-change" is stored as "before-change-<stamp>",
// so looking it up under the name the user typed has to find it. Without this
// the documented round trip (snapshot, change, restore) never resolves.
func TestResolveSnapshotNameFindsStampedName(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	seedSnapshot(t, Snapshot{Name: "before-change-20260101-101010", Created: time.Now().UTC().Add(-time.Hour), Service: "mysql", Family: "mysql", Database: "demo"})

	got, err := resolveSnapshotName("mysql", "demo", "before-change", false)
	if err != nil {
		t.Fatalf("resolveSnapshotName: %v", err)
	}
	if got != "before-change-20260101-101010" {
		t.Fatalf("got %q, want the stamped name", got)
	}
}

// Repeated snapshots of one label are supported, so the label resolves to the
// most recent of them rather than an arbitrary one.
func TestResolveSnapshotNamePicksNewest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	seedSnapshot(t, Snapshot{Name: "nightly-20260101-010101", Created: time.Now().UTC().Add(-48 * time.Hour), Service: "mysql", Family: "mysql", Database: "demo"})
	seedSnapshot(t, Snapshot{Name: "nightly-20260103-030303", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "demo"})

	got, err := resolveSnapshotName("mysql", "demo", "nightly", false)
	if err != nil {
		t.Fatalf("resolveSnapshotName: %v", err)
	}
	if got != "nightly-20260103-030303" {
		t.Fatalf("got %q, want the newest stamped name", got)
	}
}

// An exact directory name still wins, so nothing that already worked changes.
func TestResolveSnapshotNamePrefersExactMatch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	seedSnapshot(t, Snapshot{Name: "keep", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "demo"})
	seedSnapshot(t, Snapshot{Name: "keep-20260105-050505", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "demo"})

	got, err := resolveSnapshotName("mysql", "demo", "keep", false)
	if err != nil {
		t.Fatalf("resolveSnapshotName: %v", err)
	}
	if got != "keep" {
		t.Fatalf("got %q, want the exact name", got)
	}
}

// A label matching nothing is still an error, and names what was asked for.
func TestResolveSnapshotNameMissing(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if _, err := resolveSnapshotName("mysql", "demo", "absent", false); err == nil {
		t.Fatal("expected an error for a name with no snapshot behind it")
	}
}

// The stamped lookup reaches DeleteSnapshot too, so removing a snapshot by the
// name it was created under works.
func TestDeleteSnapshotByCreatedName(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	seedSnapshot(t, Snapshot{Name: "drop-me-20260101-101010", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "demo"})

	if err := DeleteSnapshot("mysql", "demo", "drop-me", false); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if _, err := os.Stat(snapshotDir("mysql", "demo", "drop-me-20260101-101010", false)); !os.IsNotExist(err) {
		t.Error("snapshot dir still present after delete")
	}
}
