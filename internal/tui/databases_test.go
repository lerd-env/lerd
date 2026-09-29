package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/dbview"
	"github.com/geodro/lerd/internal/serviceops"
)

// fakeEngines is the loaded listing the Databases pane renders from: a running
// engine with two databases (one snapshotted, one owned by a worktree) and a
// stopped one.
func fakeEngines() []dbview.Engine {
	created := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	return []dbview.Engine{
		{
			Service: "mysql", Family: "mysql", Running: true, SupportsSnapshot: true,
			Databases: []dbview.Entry{
				{
					Name: "shop", SizeBytes: 5 << 20,
					Owner:     dbview.Owner{Domain: "shop.test"},
					Snapshots: []serviceops.Snapshot{{Name: "before-migrate", Created: created, SizeBytes: 1 << 20, GitBranch: "main"}},
				},
				{Name: "shop_staging", SizeBytes: 2 << 20, Owner: dbview.Owner{Domain: "shop.test", Branch: "staging"}},
			},
		},
		{Service: "postgres", Family: "postgres"},
	}
}

func databasesModel() *Model {
	m := NewModel("test")
	m.width, m.height = 150, 40
	m.activeTab = tabDatabases
	m.focus = paneDatabases
	m.dbEngines = fakeEngines()
	m.dbLoaded = true
	return m
}

func TestDatabasesPane_ListsEnginesWithTheirDatabases(t *testing.T) {
	m := databasesModel()
	out := stripANSI(m.renderDatabases(60, 20))
	for _, want := range []string{"mysql", "shop", "shop_staging", "5MB", "postgres", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the databases pane:\n%s", want, out)
		}
	}
}

func TestDatabasesPane_EmptyStatePointsAtThePreset(t *testing.T) {
	m := NewModel("test")
	m.activeTab = tabDatabases
	m.dbLoaded = true
	out := stripANSI(m.renderDatabases(60, 20))
	if !strings.Contains(out, "no database engine installed") || !strings.Contains(out, "lerd service preset mysql") {
		t.Errorf("expected an empty state that says what to do:\n%s", out)
	}
}

func TestDatabasesCursor_SkipsEngineHeaders(t *testing.T) {
	m := databasesModel()
	if _, db := m.currentDatabase(); db == nil || db.Name != "shop" {
		t.Fatalf("expected the first database selected, got %+v", db)
	}
	m.moveCursor(1)
	_, db := m.currentDatabase()
	if db == nil || db.Name != "shop_staging" {
		t.Fatalf("expected the second database after one move, got %+v", db)
	}
	// Only two databases exist, so moving further must not walk onto the
	// stopped engine's header row.
	m.moveCursor(5)
	if _, db := m.currentDatabase(); db == nil || db.Name != "shop_staging" {
		t.Fatalf("cursor should clamp to the last database row, got %+v", db)
	}
}

func TestDatabaseDetail_ShowsOwnerSizeAndSnapshots(t *testing.T) {
	m := databasesModel()
	out := stripANSI(strings.Join(databaseDetailContentLines(m, 120), "\n"))
	for _, want := range []string{"shop", "mysql", "5MB", "shop.test", "before-migrate", "1MB", "main"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the database detail:\n%s", want, out)
		}
	}
}

func TestDatabaseDetail_NamesTheWorktreeBranch(t *testing.T) {
	m := databasesModel()
	m.moveCursor(1)
	out := stripANSI(strings.Join(databaseDetailContentLines(m, 120), "\n"))
	if !strings.Contains(out, "branch staging") {
		t.Errorf("expected the owning worktree branch:\n%s", out)
	}
}

func TestDatabaseDetail_KeepsDestructiveOpsInTheCLI(t *testing.T) {
	m := databasesModel()
	out := stripANSI(strings.Join(databaseDetailContentLines(m, 120), "\n"))
	if !strings.Contains(out, "n snapshot") {
		t.Errorf("expected the snapshot quick action:\n%s", out)
	}
	if !strings.Contains(out, "lerd db:restore") || !strings.Contains(out, "lerd db:import") {
		t.Errorf("expected restore and import to be named as CLI-only:\n%s", out)
	}
}

func TestDatabaseSnapshot_RefusesWhenTheEngineDeclaresNone(t *testing.T) {
	m := databasesModel()
	m.dbEngines[0].SupportsSnapshot = false
	if cmd := m.actionDatabaseSnapshot(); cmd != nil {
		t.Fatal("an engine without snapshots should run nothing")
	}
	if !strings.Contains(m.status, "declares no snapshots") {
		t.Errorf("expected the status to say why, got %q", m.status)
	}
}

func TestDatabaseSnapshot_RunsForTheSelectedDatabase(t *testing.T) {
	m := databasesModel()
	if cmd := m.actionDatabaseSnapshot(); cmd == nil {
		t.Fatal("expected a snapshot command for the selected database")
	}
	if !strings.Contains(m.status, "shop") {
		t.Errorf("expected the status to name the database, got %q", m.status)
	}
}

func TestEnsureDatabases_LoadsOnceAndOnlyOnItsTab(t *testing.T) {
	m := NewModel("test")
	if cmd := m.ensureDatabases(); cmd != nil {
		t.Fatal("the listing must not run outside the Databases tab")
	}
	m.activeTab = tabDatabases
	if cmd := m.ensureDatabases(); cmd == nil {
		t.Fatal("expected the listing to start on arrival")
	}
	if cmd := m.ensureDatabases(); cmd != nil {
		t.Fatal("a second call while in flight should not start another listing")
	}
	next, _ := m.Update(databasesMsg{engines: fakeEngines()})
	m = next.(*Model)
	if !m.dbLoaded || m.dbLoading || len(m.dbEngines) != 2 {
		t.Fatalf("expected the result folded in, got loaded=%v loading=%v engines=%d", m.dbLoaded, m.dbLoading, len(m.dbEngines))
	}
	if cmd := m.ensureDatabases(); cmd != nil {
		t.Fatal("a loaded listing should be reused until a manual refresh")
	}
}

func TestDatabasesTab_RendersListAndDetail(t *testing.T) {
	m := databasesModel()
	out := stripANSI(m.render())
	for _, want := range []string{"Databases", "shop", "Snapshots", "before-migrate"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the rendered Databases tab:\n%s", want, out)
		}
	}
}

func TestDatabasesTab_FocusCyclesListAndDetail(t *testing.T) {
	m := databasesModel()
	if got := m.nextFocus(+1); got != paneDetail {
		t.Fatalf("tab from the list should reach the detail pane, got %d", got)
	}
	m.focus = paneDetail
	if got := m.nextFocus(+1); got != paneDatabases {
		t.Fatalf("tab from the detail pane should return to the list, got %d", got)
	}
}

func TestDatabasesPane_ScrollsToTheSelectedRow(t *testing.T) {
	m := databasesModel()
	var many []dbview.Entry
	for i := 0; i < 40; i++ {
		many = append(many, dbview.Entry{Name: fmt.Sprintf("db%02d", i)})
	}
	m.dbEngines = []dbview.Engine{{Service: "mysql", Running: true, Databases: many}}
	m.setCursor(1 << 30)
	out := stripANSI(m.renderDatabases(60, 12))
	if !strings.Contains(out, "db39") {
		t.Errorf("expected the pane to scroll to the selected last row:\n%s", out)
	}
}

func TestDatabaseSnapshotDir_FallsBackToHomeWithoutAnOwner(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir in this environment")
	}
	if got := databaseSnapshotDir(dbview.Owner{}); got != home {
		t.Fatalf("an unowned database should snapshot from the home dir, got %q", got)
	}
}

func TestDatabases_ActionResultRelistsWithoutBlanking(t *testing.T) {
	m := databasesModel()
	next, cmd := m.Update(ActionResult{Summary: "lerd db:snapshot"})
	m = next.(*Model)
	if cmd == nil || !m.dbLoading {
		t.Fatal("a finished action on the Databases tab should re-list the engines")
	}
	if !m.dbLoaded || len(m.dbEngines) != 2 {
		t.Fatal("the held listing must stay on screen while the new one loads")
	}
}

func TestDatabases_ActionResultElsewhereLeavesTheListingAlone(t *testing.T) {
	m := databasesModel()
	m.activeTab = tabServices
	next, _ := m.Update(ActionResult{Summary: "lerd service restart redis"})
	if next.(*Model).dbLoading {
		t.Fatal("an action on another tab must not trigger container queries")
	}
}
