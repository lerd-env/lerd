package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func writeDBSiteEnv(t *testing.T, dir, host, database string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "DB_HOST=" + host + "\nDB_DATABASE=" + database + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// A worktree's isolated database has no site of its own to be found through, so
// without the registry lookup it shows as an unattached database and reads as
// stray data nobody owns.
func TestDatabaseSiteIndex_IsolatedWorktreeDBCarriesItsBranch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(t.TempDir(), "app")
	writeDBSiteEnv(t, site, "lerd-mysql", "astrolov")
	if err := config.AddSite(config.Site{Name: "astrolov", Path: site, Domains: []string{"astrolov.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := config.AddWorktreeDB(config.WorktreeDBEntry{
		Site: "astrolov", Branch: "staging", Service: "mysql", DBName: "astrolov_staging",
	}); err != nil {
		t.Fatal(err)
	}

	idx := databaseSiteIndex("mysql")

	if got := idx["astrolov"]; got.domain != "astrolov.test" || got.branch != "" {
		t.Errorf("parent database = %+v, want domain astrolov.test with no branch", got)
	}
	for _, name := range []string{"astrolov_staging", "astrolov_staging_testing"} {
		got := idx[name]
		if got.domain != "astrolov.test" || got.branch != "staging" {
			t.Errorf("%s = %+v, want domain astrolov.test on branch staging", name, got)
		}
	}
}

// A worktree DB recorded against another engine must not surface on this one.
func TestDatabaseSiteIndex_IgnoresWorktreeDBsOnAnotherService(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(t.TempDir(), "app")
	writeDBSiteEnv(t, site, "lerd-mysql", "astrolov")
	if err := config.AddSite(config.Site{Name: "astrolov", Path: site, Domains: []string{"astrolov.test"}}); err != nil {
		t.Fatal(err)
	}
	if err := config.AddWorktreeDB(config.WorktreeDBEntry{
		Site: "astrolov", Branch: "staging", Service: "postgres", DBName: "astrolov_staging",
	}); err != nil {
		t.Fatal(err)
	}

	if got, ok := databaseSiteIndex("mysql")["astrolov_staging"]; ok {
		t.Errorf("postgres worktree database surfaced on mysql: %+v", got)
	}
}
