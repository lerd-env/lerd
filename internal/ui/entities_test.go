package ui

import "testing"

// The databases kind has its own endpoints, which sanitize a dump on the way
// in, honour the fresh option and gate on the service being a database engine.
// Routing it through the generic entity endpoints would step around all of
// that, so every generic endpoint refuses it, not just the action one.
func TestGenericEntityKindExcludesDatabases(t *testing.T) {
	for _, kind := range []string{"databases", ""} {
		if genericEntityKind(kind) {
			t.Errorf("genericEntityKind(%q) = true, want false", kind)
		}
	}
	for _, kind := range []string{"buckets", "keyspaces", "indexes"} {
		if !genericEntityKind(kind) {
			t.Errorf("genericEntityKind(%q) = false, want true", kind)
		}
	}
}

// The streaming pair has its own endpoints, and the service-wide variants back
// snapshots and take no entity name, so neither may run as a row action.
func TestRowEntityActionExcludesStreamingAndServiceWide(t *testing.T) {
	for _, action := range []string{"export", "import", "export_all", "import_all", ""} {
		if rowEntityAction(action) {
			t.Errorf("rowEntityAction(%q) = true, want false", action)
		}
	}
	for _, action := range []string{"create", "delete", "flush", "purge"} {
		if !rowEntityAction(action) {
			t.Errorf("rowEntityAction(%q) = false, want true", action)
		}
	}
}

// The declared actions arrive as a map; the row renders them in a stable
// order with anything destructive at the end.
func TestSortEntityActions(t *testing.T) {
	actions := []entityActionResponse{
		{Name: "delete", Destructive: true},
		{Name: "flush"},
		{Name: "import"},
		{Name: "create"},
		{Name: "export"},
	}
	sortEntityActions(actions)
	want := []string{"create", "export", "import", "flush", "delete"}
	for i, w := range want {
		if actions[i].Name != w {
			t.Fatalf("order = %v, want %v", actions, want)
		}
	}
}

// A database with no snapshots must still carry a list, not null: the field is
// typed as an array on the client and a null breaks iteration over it.
func TestDatabaseEntryAlwaysCarriesASnapshotList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	eng := databaseEngine("nosuchengine")
	for _, db := range eng.Databases {
		if db.Snapshots == nil {
			t.Errorf("database %q carries a nil snapshot list", db.Name)
		}
	}
	if eng.Databases == nil {
		t.Error("engine carries a nil database list")
	}
}
