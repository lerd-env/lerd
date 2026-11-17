package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The site list tells plugins which databases a site points at, so a database
// browser can offer them without guessing from service names.
func TestBuildSites_CarriesTheSiteDatabases(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("db:\n  service: postgres\n  database: shop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatal(err)
	}
	sites, err := buildSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || !sites[0].HasDatabase || len(sites[0].Databases) != 1 || sites[0].Databases[0] != (siteDatabase{Service: "postgres", Database: "shop"}) {
		t.Fatalf("sites = %+v, want shop with its postgres database", sites)
	}
}

// addWorktree lays out a git worktree of main the way git does, so lerd finds
// it, and returns its checkout.
func addWorktree(t *testing.T, main, branch string) string {
	t.Helper()
	meta := filepath.Join(main, ".git", "worktrees", branch)
	checkout := filepath.Join(t.TempDir(), branch)
	for _, dir := range []string{meta, checkout} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(meta, "HEAD"):   "ref: refs/heads/" + branch + "\n",
		filepath.Join(meta, "gitdir"): filepath.Join(checkout, ".git") + "\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return checkout
}

// A plugin tab's when reads the site's framework major and each worktree's own
// database, so the site list carries both.
func TestBuildSites_CarriesWhatATabConditionReads(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	// A cached definition, so the major resolves without reaching the store.
	store := config.StoreFrameworksDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "laravel@12.yaml"), []byte("name: laravel\nlabel: Laravel\nversion: \"12\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := t.TempDir()
	if err := os.WriteFile(filepath.Join(main, "composer.json"), []byte(`{"require":{"laravel/framework":"^12.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	checkout := addWorktree(t, main, "feat")
	if err := os.WriteFile(filepath.Join(checkout, ".lerd.yaml"), []byte("db:\n  service: postgres\n  database: feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: main, Framework: "laravel"}); err != nil {
		t.Fatal(err)
	}
	sites, err := buildSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 {
		t.Fatalf("sites = %+v, want shop", sites)
	}
	s := sites[0]
	if s.FrameworkVersion != "12" {
		t.Errorf("FrameworkVersion = %q, want 12", s.FrameworkVersion)
	}
	if s.HasDatabase || len(s.Worktrees) != 1 || !s.Worktrees[0].HasDatabase {
		t.Errorf("site has_database = %v, worktrees = %+v; want only the worktree to have one", s.HasDatabase, s.Worktrees)
	}
}
