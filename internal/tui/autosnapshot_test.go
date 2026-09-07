package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dbview"
	"github.com/geodro/lerd/internal/serviceops"
)

func TestNextAutoSnapshotMode_cycles(t *testing.T) {
	next, label := nextAutoSnapshotMode(config.AutoSnapshotDefault)
	if next != config.AutoSnapshotOn || label != "always" {
		t.Errorf("from default: got %q/%q", next, label)
	}
	if next, _ = nextAutoSnapshotMode(config.AutoSnapshotOn); next != config.AutoSnapshotOff {
		t.Errorf("from on: got %q, want off", next)
	}
	// Back to following the policy, which the CLI spells "default".
	if next, _ = nextAutoSnapshotMode(config.AutoSnapshotOff); next != "default" {
		t.Errorf("from off: got %q, want default", next)
	}
}

func TestAutoSnapshotSettingLabel(t *testing.T) {
	cfg := &config.GlobalConfig{}
	if got := autoSnapshotSettingLabel(cfg); got != "Automatic database snapshots" {
		t.Errorf("disabled label = %q", got)
	}
	cfg.AutoSnapshot.Enabled = true
	cfg.AutoSnapshot.Every = "6h"
	cfg.AutoSnapshot.Keep = 5
	want := "Automatic database snapshots (every 6h, keeping 5, opted-in sites)"
	if got := autoSnapshotSettingLabel(cfg); got != want {
		t.Errorf("enabled label = %q, want %q", got, want)
	}

	// Opt-out changes what the row promises, so it says so.
	cfg.AutoSnapshot.Selection = config.AutoSnapshotOptOut
	if got := autoSnapshotSettingLabel(cfg); got != "Automatic database snapshots (every 6h, keeping 5, all sites)" {
		t.Errorf("opt-out label = %q", got)
	}
}

// snapshotKeepModel is a Databases pane whose selected database holds one manual
// and two automatic snapshots.
func snapshotKeepModel(t *testing.T) *Model {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	now := time.Now().UTC()
	m := NewModel("test")
	m.width, m.height = 150, 40
	m.activeTab = tabDatabases
	m.focus = paneDatabases
	m.dbLoaded = true
	m.dbEngines = []dbview.Engine{{
		Service: "mysql", Family: "mysql", Running: true, SupportsSnapshot: true,
		Databases: []dbview.Entry{{
			Name: "shop",
			Snapshots: []serviceops.Snapshot{
				{Name: "auto-new", Created: now, Service: "mysql", Database: "shop", Auto: true},
				{Name: "auto-kept", Created: now.Add(-time.Hour), Service: "mysql", Database: "shop", Auto: true, Kept: true},
				{Name: "by-hand", Created: now.Add(-2 * time.Hour), Service: "mysql", Database: "shop"},
			},
		}},
	}}
	return m
}

// Retention never touches a manual snapshot, so offering to keep one would be
// offering nothing.
func TestOpenSnapshotKeepPicker_listsOnlyAutomaticSnapshots(t *testing.T) {
	m := snapshotKeepModel(t)
	m.openSnapshotKeepPicker()

	if m.pickerKind != kindSnapshotKeep {
		t.Fatalf("picker kind = %v, want the keep picker", m.pickerKind)
	}
	if len(m.pickerSnapshotNames) != 2 {
		t.Fatalf("names = %v, want the two automatic snapshots", m.pickerSnapshotNames)
	}
	if m.pickerSnapshotNames[0] != "auto-new" || m.pickerSnapshotNames[1] != "auto-kept" {
		t.Errorf("names = %v", m.pickerSnapshotNames)
	}
	if m.pickerSnapshotKept[0] || !m.pickerSnapshotKept[1] {
		t.Errorf("kept flags = %v, want [false true]", m.pickerSnapshotKept)
	}
	if !strings.Contains(m.pickerOptions[1], "kept") {
		t.Errorf("a kept snapshot should say so: %q", m.pickerOptions[1])
	}
	if m.pickerSnapshotService != "mysql" || m.pickerSnapshotDB != "shop" {
		t.Errorf("target = %s/%s", m.pickerSnapshotService, m.pickerSnapshotDB)
	}
}

func TestOpenSnapshotKeepPicker_noAutomaticSnapshots(t *testing.T) {
	m := snapshotKeepModel(t)
	m.dbEngines[0].Databases[0].Snapshots = []serviceops.Snapshot{{Name: "by-hand", Service: "mysql", Database: "shop"}}
	m.openSnapshotKeepPicker()

	if m.pickerKind == kindSnapshotKeep {
		t.Error("the picker should stay closed when there is nothing to keep")
	}
	if !strings.Contains(m.status, "no automatic snapshots") {
		t.Errorf("status = %q", m.status)
	}
}

// Applying closes the picker and leaves no target behind for the next open.
func TestApplySnapshotKeepPicker_clearsState(t *testing.T) {
	m := snapshotKeepModel(t)
	m.openSnapshotKeepPicker()
	if cmd := m.applySnapshotKeepPicker(); cmd == nil {
		t.Fatal("expected a command running the keep verb")
	}
	if m.pickerKind == kindSnapshotKeep || m.pickerSnapshotNames != nil || m.pickerSnapshotService != "" {
		t.Errorf("picker state survived the apply: %+v", m.pickerSnapshotNames)
	}
}
