package serviceops

import (
	"os"
	"testing"
	"time"
)

// autoSnap builds an automatic snapshot of myapp created n hours before base.
func autoSnap(name string, created time.Time) Snapshot {
	return Snapshot{
		Name: name, Created: created, Service: "mysql", Family: "mysql",
		Database: "myapp", DumpFile: snapshotDumpFile, Compressed: true, Auto: true,
	}
}

func TestSnapshotExpiries_countRule(t *testing.T) {
	base := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	// Newest first, one a day apart.
	snaps := []Snapshot{
		autoSnap("d0", base),
		autoSnap("d1", base.Add(-24*time.Hour)),
		autoSnap("d2", base.Add(-48*time.Hour)),
	}
	p := RetentionPolicy{Keep: 3, Every: 24 * time.Hour}

	got := SnapshotExpiries(p, snaps)
	if len(got) != len(snaps) {
		t.Fatalf("got %d expiries, want %d", len(got), len(snaps))
	}
	wantRuns := []int{3, 2, 1}
	for i, want := range wantRuns {
		if got[i].RunsLeft != want {
			t.Errorf("%s: RunsLeft = %d, want %d", snaps[i].Name, got[i].RunsLeft, want)
		}
		if !got[i].Estimated {
			t.Errorf("%s: a count-derived expiry is an estimate", snaps[i].Name)
		}
		wantAt := base.Add(time.Duration(want) * 24 * time.Hour)
		if !got[i].At.Equal(wantAt) {
			t.Errorf("%s: At = %v, want %v", snaps[i].Name, got[i].At, wantAt)
		}
	}
}

// A manual snapshot is never pruned, so it carries no expiry at all, and a kept
// one reports itself as kept rather than as a date.
func TestSnapshotExpiries_manualAndKept(t *testing.T) {
	base := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	manual := Snapshot{Name: "pre-migration", Created: base, Service: "mysql", Database: "myapp"}
	kept := autoSnap("keeper", base.Add(-time.Hour))
	kept.Kept = true
	snaps := []Snapshot{manual, kept, autoSnap("plain", base.Add(-2*time.Hour))}

	got := SnapshotExpiries(RetentionPolicy{Keep: 1, Every: time.Hour}, snaps)
	if got[0] != (SnapshotExpiry{}) {
		t.Errorf("manual snapshot: got %+v, want no expiry", got[0])
	}
	if !got[1].Kept || !got[1].At.IsZero() {
		t.Errorf("kept snapshot: got %+v, want kept with no date", got[1])
	}
	// The kept one takes no slot, so the plain snapshot is still the newest of
	// its window rather than already over it.
	if got[2].RunsLeft != 1 {
		t.Errorf("plain snapshot: RunsLeft = %d, want 1 (a kept snapshot takes no slot)", got[2].RunsLeft)
	}
}

func TestSnapshotExpiries_ageRuleWinsWhenEarlier(t *testing.T) {
	base := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	snaps := []Snapshot{autoSnap("only", base)}
	p := RetentionPolicy{Keep: 30, KeepFor: 48 * time.Hour, Every: 24 * time.Hour}

	got := SnapshotExpiries(p, snaps)
	wantAt := base.Add(48 * time.Hour)
	if !got[0].At.Equal(wantAt) {
		t.Errorf("At = %v, want the age cutoff %v", got[0].At, wantAt)
	}
	if got[0].Estimated {
		t.Error("an age-derived expiry is exact, not an estimate")
	}
}

// Two databases each get their own retention window, so a busy database can't
// push another one's snapshots out.
func TestSnapshotExpiries_perDatabaseWindows(t *testing.T) {
	base := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	shop := autoSnap("shop-1", base)
	shop.Database = "shop"
	snaps := []Snapshot{autoSnap("myapp-1", base), shop}

	got := SnapshotExpiries(RetentionPolicy{Keep: 1, Every: time.Hour}, snaps)
	for i := range snaps {
		if got[i].RunsLeft != 1 {
			t.Errorf("%s: RunsLeft = %d, want 1", snaps[i].Name, got[i].RunsLeft)
		}
	}
}

func TestSnapshotExpiries_noLimitsNeverExpire(t *testing.T) {
	got := SnapshotExpiries(RetentionPolicy{Every: time.Hour}, []Snapshot{autoSnap("forever", time.Now().UTC())})
	if !got[0].At.IsZero() || got[0].RunsLeft != 0 {
		t.Errorf("with no limits configured, got %+v, want no expiry", got[0])
	}
}

func TestPruneAutoSnapshots(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	now := time.Now().UTC()

	manual := Snapshot{Name: "pre-migration", Created: now.Add(-96 * time.Hour), Service: "mysql", Family: "mysql", Database: "myapp"}
	kept := autoSnap("kept-old", now.Add(-96*time.Hour))
	kept.Kept = true
	newest := autoSnap("auto-1", now.Add(-time.Hour))
	middle := autoSnap("auto-2", now.Add(-2*time.Hour))
	oldest := autoSnap("auto-3", now.Add(-3*time.Hour))
	for _, s := range []Snapshot{manual, kept, newest, middle, oldest} {
		seedSnapshot(t, s)
	}

	removed, err := PruneAutoSnapshots("mysql", "myapp", RetentionPolicy{Keep: 2})
	if err != nil {
		t.Fatalf("PruneAutoSnapshots: %v", err)
	}
	if len(removed) != 1 || removed[0] != "auto-3" {
		t.Fatalf("removed = %v, want [auto-3]", removed)
	}
	if _, err := os.Stat(snapshotDir("mysql", "myapp", "auto-3", false)); !os.IsNotExist(err) {
		t.Error("the expired snapshot is still on disk")
	}
	for _, name := range []string{"pre-migration", "kept-old", "auto-1", "auto-2"} {
		if _, err := os.Stat(snapshotDir("mysql", "myapp", name, false)); err != nil {
			t.Errorf("%s should have survived: %v", name, err)
		}
	}
}

func TestPruneAutoSnapshots_ageRule(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	now := time.Now().UTC()
	seedSnapshot(t, autoSnap("fresh", now.Add(-time.Hour)))
	seedSnapshot(t, autoSnap("stale", now.Add(-72*time.Hour)))

	removed, err := PruneAutoSnapshots("mysql", "myapp", RetentionPolicy{KeepFor: 48 * time.Hour})
	if err != nil {
		t.Fatalf("PruneAutoSnapshots: %v", err)
	}
	if len(removed) != 1 || removed[0] != "stale" {
		t.Fatalf("removed = %v, want [stale]", removed)
	}
}

func TestSetSnapshotKept(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	seedSnapshot(t, autoSnap("auto-1", time.Now().UTC()))
	seedSnapshot(t, Snapshot{Name: "manual", Created: time.Now().UTC(), Service: "mysql", Family: "mysql", Database: "myapp"})

	if err := SetSnapshotKept("mysql", "myapp", "auto-1", false, true); err != nil {
		t.Fatalf("SetSnapshotKept: %v", err)
	}
	snaps, err := ListSnapshots("mysql", "myapp", false)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	for _, s := range snaps {
		if s.Name == "auto-1" && !s.Kept {
			t.Error("auto-1 should be kept")
		}
	}
	// Releasing it puts it back under retention.
	if err := SetSnapshotKept("mysql", "myapp", "auto-1", false, false); err != nil {
		t.Fatalf("release: %v", err)
	}
	snaps, _ = ListSnapshots("mysql", "myapp", false)
	for _, s := range snaps {
		if s.Name == "auto-1" && s.Kept {
			t.Error("auto-1 should be back under retention")
		}
	}

	// A manual snapshot is already permanent, so asking to keep it is a mistake
	// worth naming rather than a silent no-op.
	if err := SetSnapshotKept("mysql", "myapp", "manual", false, true); err == nil {
		t.Error("keeping a manual snapshot should be rejected")
	}
	if err := SetSnapshotKept("mysql", "myapp", "missing", false, true); err == nil {
		t.Error("an unknown snapshot should be rejected")
	}
}
