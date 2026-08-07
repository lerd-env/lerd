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

// A WordPress site holds no .env at all: its database is named by DB_NAME in
// wp-config.php, which the framework definition publishes. Without reading it
// through the declaration the site is listed, the database is listed, and the
// card never connects the two.
func TestDatabaseSiteIndex_WordPressSiteOwnsItsDatabase(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	storeDir := config.StoreFrameworksDir()
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}
	def := "name: wordpress\nlabel: WordPress\nenv:\n  fallback_file: wp-config.php\n  fallback_format: php-const\n  services:\n    mysql:\n      detect:\n        - key: DB_HOST\n      vars:\n        - DB_NAME={{site}}\n        - DB_HOST=lerd-mysql\n"
	if err := os.WriteFile(filepath.Join(storeDir, "wordpress.yaml"), []byte(def), 0644); err != nil {
		t.Fatal(err)
	}

	site := filepath.Join(t.TempDir(), "blog")
	if err := os.MkdirAll(site, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, ".lerd.yaml"), []byte("framework: wordpress\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wpConfig := "<?php\ndefine( 'DB_NAME', 'blog' );\ndefine( 'DB_HOST', 'lerd-mysql' );\n"
	if err := os.WriteFile(filepath.Join(site, "wp-config.php"), []byte(wpConfig), 0644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "blog", Path: site, Domains: []string{"blog.test"}}); err != nil {
		t.Fatal(err)
	}

	if got := databaseSiteIndex("mysql")["blog"]; got.domain != "blog.test" {
		t.Errorf("blog database = %+v, want domain blog.test", got)
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
