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

	"github.com/geodro/lerd/internal/cli"
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

// Pull targets the checkout the tab shows: a worktree's branch moves, main's does not.
func TestHandleSitePull_worktree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	remote, sitePath, other := t.TempDir(), t.TempDir(), t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	gitIn(t, sitePath, "commit", "-q", "--allow-empty", "-m", "init")
	gitIn(t, sitePath, "remote", "add", "origin", remote)
	gitIn(t, sitePath, "push", "-q", "origin", "main", "main:feature")
	gitIn(t, sitePath, "fetch", "-q")
	wtPath := filepath.Join(t.TempDir(), "feature")
	gitIn(t, sitePath, "worktree", "add", "-q", "--track", "-b", "feature", wtPath, "origin/feature")
	gitIn(t, other, "clone", "-q", "-b", "feature", remote, ".")
	gitIn(t, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	gitIn(t, other, "push", "-q")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	mainBefore := runGitOutput(sitePath, "rev-parse", "HEAD")

	rec := httptest.NewRecorder()
	target := reviewedTarget(t, "feature")
	handleSitePull(rec, httptest.NewRequest(http.MethodPost, "/api/sites/pull?domain=acme.test&branch=feature&target="+target, nil))

	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("want ok, got %s", rec.Body.String())
	}
	if got, want := runGitOutput(wtPath, "rev-parse", "HEAD"), runGitOutput(other, "rev-parse", "HEAD"); got != want {
		t.Errorf("worktree not fast-forwarded: %s want %s", got, want)
	}
	if runGitOutput(sitePath, "rev-parse", "HEAD") != mainBefore {
		t.Error("main checkout moved")
	}
}

func TestHandleSiteAction_gitPushMain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	remote, sitePath := t.TempDir(), t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	gitIn(t, sitePath, "commit", "-q", "--allow-empty", "-m", "init")
	gitIn(t, sitePath, "remote", "add", "origin", remote)
	gitIn(t, sitePath, "push", "-q", "-u", "origin", "main")
	gitIn(t, sitePath, "commit", "-q", "--allow-empty", "-m", "ours")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteAction(rec, httptest.NewRequest(http.MethodPost, "/api/sites/acme.test/git:push", nil))

	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("want ok, got %s", rec.Body.String())
	}
	if runGitOutput(remote, "rev-parse", "main") != runGitOutput(sitePath, "rev-parse", "HEAD") {
		t.Error("remote did not receive the commit")
	}
}

func TestHandleSiteBranchSwitch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	gitIn(t, sitePath, "commit", "-q", "--allow-empty", "-m", "init")
	gitIn(t, sitePath, "branch", "dev")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteBranchSwitch(rec, httptest.NewRequest(http.MethodPost, "/api/sites/branch-switch?domain=acme.test&branch=dev", nil))

	if !strings.Contains(rec.Body.String(), "event: done") || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("want a done event with ok, got %s", rec.Body.String())
	}
	if got := strings.TrimSpace(runGitOutput(sitePath, "symbolic-ref", "--short", "HEAD")); got != "dev" {
		t.Errorf("on %q, want dev", got)
	}
}

func TestHandleSiteBranchSwitch_refusal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	gitIn(t, sitePath, "commit", "-q", "--allow-empty", "-m", "init")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteBranchSwitch(rec, httptest.NewRequest(http.MethodPost, "/api/sites/branch-switch?domain=acme.test&branch=nope", nil))

	if !strings.Contains(rec.Body.String(), `"ok":false`) || !strings.Contains(rec.Body.String(), "nope") {
		t.Fatalf("want git's refusal naming the branch, got %s", rec.Body.String())
	}
}

func TestHandleSiteBranchPlan(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(sitePath, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sitePath, "add", ".")
	gitIn(t, sitePath, "commit", "-q", "-m", "init")
	gitIn(t, sitePath, "checkout", "-q", "-b", "dev")
	if err := os.WriteFile(filepath.Join(sitePath, "composer.lock"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sitePath, "add", ".")
	gitIn(t, sitePath, "commit", "-q", "-m", "lock")
	gitIn(t, sitePath, "checkout", "-q", "main")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteBranchPlan(rec, httptest.NewRequest(http.MethodGet, "/api/sites/branch-plan?domain=acme.test&branch=dev", nil))

	body := rec.Body.String()
	if !strings.Contains(body, `"changed":"composer.lock"`) || !strings.Contains(body, `"behind":1`) {
		t.Fatalf("want composer due for the changed lockfile, got %s", body)
	}
}

func TestHandleSiteBranchSwitch_createsBranch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	gitIn(t, sitePath, "commit", "-q", "--allow-empty", "-m", "init")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleSiteBranchSwitch(rec, httptest.NewRequest(http.MethodPost, "/api/sites/branch-switch?domain=acme.test&branch=feature%2Fnew&create=1", nil))

	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("want ok, got %s", rec.Body.String())
	}
	if got := strings.TrimSpace(runGitOutput(sitePath, "symbolic-ref", "--short", "HEAD")); got != "feature/new" {
		t.Errorf("on %q, want feature/new", got)
	}
}

// A pull fast-forwards files, so it waits out a switch on the same checkout.
func TestHandleSitePull_waitsForASwitch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath := t.TempDir()
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	release := cli.CheckoutLock(sitePath)
	defer release()

	rec := httptest.NewRecorder()
	handleSitePull(rec, httptest.NewRequest(http.MethodPost, "/api/sites/pull?domain=acme.test", nil))

	if !strings.Contains(rec.Body.String(), "branch switch is running") {
		t.Fatalf("want a refusal while a switch runs, got %s", rec.Body.String())
	}
}

// pullSite links acme.test at a checkout whose remote has one commit adding
// composer.lock that the checkout has not fetched yet.
func pullSite(t *testing.T) (sitePath, other string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	remote, sitePath, other := t.TempDir(), t.TempDir(), t.TempDir()
	gitIn(t, remote, "init", "-q", "--bare", "-b", "main")
	gitIn(t, sitePath, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(sitePath, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, sitePath, "add", ".")
	gitIn(t, sitePath, "commit", "-q", "-m", "init")
	gitIn(t, sitePath, "remote", "add", "origin", remote)
	gitIn(t, sitePath, "push", "-q", "-u", "origin", "main")
	gitIn(t, other, "clone", "-q", remote, ".")
	if err := os.WriteFile(filepath.Join(other, "composer.lock"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, other, "add", ".")
	gitIn(t, other, "commit", "-q", "-m", "lock")
	gitIn(t, other, "push", "-q")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	return sitePath, other
}

// reviewedTarget asks for acme.test's pull plan, as the dialog does before a
// pull, and returns the commit it reviewed.
func reviewedTarget(t *testing.T, branch string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handleSitePullPlan(rec, httptest.NewRequest(http.MethodGet, "/api/sites/pull-plan?domain=acme.test&branch="+branch, nil))
	var p struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Target == "" {
		t.Fatalf("no reviewed commit in %s", rec.Body.String())
	}
	return p.Target
}

func TestHandleSitePullPlan(t *testing.T) {
	pullSite(t)

	rec := httptest.NewRecorder()
	handleSitePullPlan(rec, httptest.NewRequest(http.MethodGet, "/api/sites/pull-plan?domain=acme.test", nil))

	body := rec.Body.String()
	if !strings.Contains(body, `"changed":"composer.lock"`) || !strings.Contains(body, `"behind":1`) {
		t.Fatalf("want composer due for the incoming lockfile, got %s", body)
	}
}

func TestHandleSitePull(t *testing.T) {
	sitePath, other := pullSite(t)

	rec := httptest.NewRecorder()
	handleSitePull(rec, httptest.NewRequest(http.MethodPost, "/api/sites/pull?domain=acme.test&target="+reviewedTarget(t, ""), nil))

	if !strings.Contains(rec.Body.String(), "event: done") || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("want a done event with ok, got %s", rec.Body.String())
	}
	if runGitOutput(sitePath, "rev-parse", "HEAD") != runGitOutput(other, "rev-parse", "HEAD") {
		t.Error("checkout not fast-forwarded")
	}
}

func TestHandleSitePull_unknownWorktree(t *testing.T) {
	pullSite(t)

	rec := httptest.NewRecorder()
	handleSitePull(rec, httptest.NewRequest(http.MethodPost, "/api/sites/pull?domain=acme.test&branch=nope", nil))

	if !strings.Contains(rec.Body.String(), "unknown worktree branch") {
		t.Fatalf("want the worktree refusal, got %s", rec.Body.String())
	}
}
