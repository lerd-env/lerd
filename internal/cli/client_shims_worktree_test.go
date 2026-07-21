package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func writeEnv(t *testing.T, dir, host string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST="+host+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestSiteServiceForTool_NestedWorktreeUsesItsOwnEnv covers a dump run from a
// worktree checked out inside its parent site. The parent matches by path prefix,
// so without resolving the worktree the shim reads the parent's DB_HOST and a
// branch dump silently comes from the parent's database.
func TestSiteServiceForTool_NestedWorktreeUsesItsOwnEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")
	writeEnv(t, site, "lerd-mysql-8.4")
	writeEnv(t, wt, "lerd-postgres-17")

	if err := config.AddSite(config.Site{Name: "app", Path: site}); err != nil {
		t.Fatal(err)
	}

	if got := siteServiceForTool(wt, "psql"); got != "postgres-17" {
		t.Errorf("siteServiceForTool = %q, want %q", got, "postgres-17")
	}
}

// TestSiteServiceForTool_SubdirOfWorktreeUsesTheWorktreeEnv resolves the checkout
// from a directory inside it, since dumps run from anywhere in the project.
func TestSiteServiceForTool_SubdirOfWorktreeUsesTheWorktreeEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")
	writeEnv(t, site, "lerd-mysql-8.4")
	writeEnv(t, wt, "lerd-postgres-17")
	sub := filepath.Join(wt, "database", "dumps")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	if err := config.AddSite(config.Site{Name: "app", Path: site}); err != nil {
		t.Fatal(err)
	}

	if got := siteServiceForTool(sub, "pg_dump"); got != "postgres-17" {
		t.Errorf("siteServiceForTool = %q, want %q", got, "postgres-17")
	}
}

// TestSiteServiceForTool_WorktreeWithoutEnvFallsBackToTheSite keeps a worktree
// that never had its env rewritten on the parent's service.
func TestSiteServiceForTool_WorktreeWithoutEnvFallsBackToTheSite(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")
	writeEnv(t, site, "lerd-mysql-8.4")

	if err := config.AddSite(config.Site{Name: "app", Path: site}); err != nil {
		t.Fatal(err)
	}

	if got := siteServiceForTool(wt, "mysqldump"); got != "mysql-8.4" {
		t.Errorf("siteServiceForTool = %q, want %q", got, "mysql-8.4")
	}
}

// TestSiteServiceForTool_PlainSiteUsesTheSiteRoot keeps the ordinary case: from a
// subdirectory of a registered site the project root's env is the one to read.
func TestSiteServiceForTool_PlainSiteUsesTheSiteRoot(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	site := filepath.Join(tempRoot(t), "app")
	sub := filepath.Join(site, "database", "dumps")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	writeEnv(t, site, "lerd-mariadb-11.4")

	if err := config.AddSite(config.Site{Name: "app", Path: site}); err != nil {
		t.Fatal(err)
	}

	if got := siteServiceForTool(sub, "mysqldump"); got != "mariadb-11.4" {
		t.Errorf("siteServiceForTool = %q, want %q", got, "mariadb-11.4")
	}
}
