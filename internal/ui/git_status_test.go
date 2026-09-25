package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Each checkout reports its own tree: a dirty main must not colour a clean worktree.
func TestHandleSiteGitStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	sitePath := t.TempDir()
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(sitePath, "a.php"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sitePath, "add", "a.php")
	gitIn(t, sitePath, "commit", "-q", "-m", "init")
	wtPath := filepath.Join(t.TempDir(), "feature")
	gitIn(t, sitePath, "worktree", "add", "-q", "-b", "feature", wtPath)
	if err := os.WriteFile(filepath.Join(sitePath, "a.php"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteGitStatus(rec, httptest.NewRequest(http.MethodGet, "/api/sites/git-status?domain=acme.test", nil))

	var resp struct {
		Checkouts []struct {
			Branch   string `json:"branch"`
			Path     string `json:"path"`
			Main     bool   `json:"main"`
			Modified int    `json:"modified"`
		} `json:"checkouts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if len(resp.Checkouts) != 2 {
		t.Fatalf("want main + one worktree, got %s", rec.Body.String())
	}
	main, wt := resp.Checkouts[0], resp.Checkouts[1]
	if !main.Main || main.Branch != "main" || main.Modified != 1 {
		t.Errorf("main checkout: %+v", main)
	}
	// The UI matches tabs by path: lerd sanitises branch names, git does not.
	if wantPath, _ := filepath.EvalSymlinks(wtPath); wt.Path != wantPath && wt.Path != wtPath {
		t.Errorf("worktree path: got %q want %q", wt.Path, wtPath)
	}
	if wt.Main || wt.Branch != "feature" || wt.Modified != 0 {
		t.Errorf("worktree: %+v", wt)
	}
}

func TestHandleSiteGitStatus_unknownSite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	rec := httptest.NewRecorder()
	handleSiteGitStatus(rec, httptest.NewRequest(http.MethodGet, "/api/sites/git-status?domain=nope.test", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSiteAction_gitInit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteAction(rec, httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/git:init", nil))

	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("want ok, got %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(sitePath, ".git", "HEAD")); err != nil {
		t.Errorf("repo not created: %v", err)
	}
}

func TestHandleSiteGitStatus_ignoredByParent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	parent := t.TempDir()
	gitIn(t, parent, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte("sites\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sitePath := filepath.Join(parent, "sites", "shop")
	if err := os.MkdirAll(sitePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: sitePath, Domains: []string{"shop.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteGitStatus(rec, httptest.NewRequest(http.MethodGet, "/api/sites/git-status?domain=shop.test", nil))

	if got := strings.TrimSpace(rec.Body.String()); got != `{"checkouts":[]}` {
		t.Fatalf("want no checkouts, got %s", got)
	}
}
