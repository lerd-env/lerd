package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// sqliteParent writes a parent project on SQLite with the given env and returns
// it as a site next to an empty worktree checkout.
func sqliteParent(t *testing.T, env string) (*config.Site, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	return &config.Site{Name: "shop", Path: parent}, t.TempDir()
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSeedWorktreeSQLite_copiesTheParentDatabaseAndItsLog(t *testing.T) {
	site, wt := sqliteParent(t, "DB_CONNECTION=sqlite\nDB_DATABASE=database/database.sqlite\n")
	writeFixture(t, filepath.Join(site.Path, "database", "database.sqlite"), "main data")
	writeFixture(t, filepath.Join(site.Path, "database", "database.sqlite-wal"), "main wal")

	seeded, err := SeedWorktreeSQLite(site, wt)
	if err != nil || !seeded {
		t.Fatalf("SeedWorktreeSQLite = %v, %v; want true, nil", seeded, err)
	}
	for file, want := range map[string]string{"database.sqlite": "main data", "database.sqlite-wal": "main wal"} {
		got, err := os.ReadFile(filepath.Join(wt, "database", file))
		if err != nil || string(got) != want {
			t.Errorf("worktree %s = %q, %v; want %q", file, got, err, want)
		}
	}
}

// A worktree that already has its database keeps it: that data is the branch's.
func TestSeedWorktreeSQLite_neverOverwritesTheWorktreeFile(t *testing.T) {
	site, wt := sqliteParent(t, "DB_CONNECTION=sqlite\n")
	writeFixture(t, filepath.Join(site.Path, "database", "database.sqlite"), "main data")
	writeFixture(t, filepath.Join(wt, "database", "database.sqlite"), "branch data")

	if seeded, err := SeedWorktreeSQLite(site, wt); err != nil || seeded {
		t.Fatalf("SeedWorktreeSQLite = %v, %v; want false, nil", seeded, err)
	}
	if got, _ := os.ReadFile(filepath.Join(wt, "database", "database.sqlite")); string(got) != "branch data" {
		t.Errorf("worktree database = %q, want the branch's own", got)
	}
}

func TestSeedWorktreeSQLite_ignoresAServerDatabase(t *testing.T) {
	site, wt := sqliteParent(t, "DB_CONNECTION=mysql\nDB_DATABASE=shop\n")

	if seeded, err := SeedWorktreeSQLite(site, wt); err != nil || seeded {
		t.Fatalf("SeedWorktreeSQLite = %v, %v; want false, nil", seeded, err)
	}
}
