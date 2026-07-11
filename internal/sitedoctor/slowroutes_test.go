package sitedoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/reqstats"
)

// seedSlowSnapshot registers a site at dir and writes a request-timing snapshot
// with one flagged route, returning the site path.
func seedSlowSnapshot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: dir, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	a := reqstats.New(func(h string) (string, bool) { return "acme", h == "acme.test" })
	for i := 0; i < 20; i++ {
		a.Record(reqstats.AccessRecord{Host: "acme.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: "/home"})
	}
	for i := 0; i < 10; i++ {
		a.Record(reqstats.AccessRecord{Host: "acme.test", Status: 200, RequestTime: 0.5, Method: "GET", URI: "/reports/7"})
	}
	if err := a.Save(config.RequestStatsFile()); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckSlowRoutes_warnsWithFlaggedRoute(t *testing.T) {
	dir := seedSlowSnapshot(t)
	c, ok := checkSlowRoutes(dir)
	if !ok {
		t.Fatal("expected a slow-routes finding")
	}
	if c.Status != StatusWarn {
		t.Errorf("status = %q, want warn", c.Status)
	}
	if !strings.Contains(c.Detail, "GET /reports/:id") {
		t.Errorf("detail should name the slow route, got %q", c.Detail)
	}
	if c.Fix != "" {
		t.Errorf("slow routes have no command fix, got %q", c.Fix)
	}
}

func TestCheckSlowRoutes_silentWhenNoSnapshot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: dir, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := checkSlowRoutes(dir); ok {
		t.Error("no snapshot must yield no finding")
	}
}

func TestCheckSlowRoutes_silentForUnregisteredPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if _, ok := checkSlowRoutes(t.TempDir()); ok {
		t.Error("a path belonging to no site must yield no finding")
	}
}

// A worktree is checked against its own traffic, not its parent's. Its path isn't
// a registered site, so the check used to resolve nothing and stay silent however
// slow the branch ran.
func TestCheckSlowRoutes_warnsForAWorktreePath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent := t.TempDir()
	wtPath := seedWorktree(t, parent, "feature-x")
	if err := config.AddSite(config.Site{Name: "acme", Path: parent, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	// Traffic on the worktree's own domain, which the watcher records under the
	// branch key. The parent site sees none.
	a := reqstats.New(func(h string) (string, bool) {
		return reqstats.Key("acme", "feature-x"), h == "feature-x.acme.test"
	})
	for i := 0; i < 20; i++ {
		a.Record(reqstats.AccessRecord{Host: "feature-x.acme.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: "/home"})
	}
	for i := 0; i < 10; i++ {
		a.Record(reqstats.AccessRecord{Host: "feature-x.acme.test", Status: 200, RequestTime: 0.5, Method: "GET", URI: "/reports/7"})
	}
	if err := a.Save(config.RequestStatsFile()); err != nil {
		t.Fatal(err)
	}

	c, ok := checkSlowRoutes(wtPath)
	if !ok {
		t.Fatal("expected a slow-routes finding for the worktree")
	}
	if !strings.Contains(c.Detail, "GET /reports/:id") {
		t.Errorf("detail should name the worktree's slow route, got %q", c.Detail)
	}
	if _, ok := checkSlowRoutes(parent); ok {
		t.Error("the parent has no traffic of its own and must stay silent")
	}
}

// seedWorktree writes the .git/worktrees metadata and checkout dir git itself
// would, so worktree detection resolves branch and path from a real layout.
func seedWorktree(t *testing.T, parent, branch string) string {
	t.Helper()
	checkout := filepath.Join(parent, filepath.Base(parent)+"-"+branch)
	entry := filepath.Join(parent, ".git", "worktrees", branch)
	if err := os.MkdirAll(entry, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitdir := filepath.Join(checkout, ".git")
	if err := os.WriteFile(filepath.Join(entry, "gitdir"), []byte(gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return checkout
}
