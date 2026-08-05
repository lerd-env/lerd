package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func TestToolJSON_wrapsValueInContent(t *testing.T) {
	result := toolJSON(map[string]any{"site": "demo", "worktrees": []string{"a"}})
	if _, has := result["isError"]; has {
		t.Error("toolJSON must not set isError on success")
	}
	var parsed map[string]any
	decodeContent(t, result, &parsed)
	if parsed["site"] != "demo" {
		t.Errorf("expected site=demo, got %v", parsed["site"])
	}
}

// TestExecWorktreeList_returnsContent guards the regression where a handler
// returned a bare map with no "content" key, so the MCP host rendered a live
// worktree as "no output". The result must carry a content block.
func TestExecWorktreeList_returnsContent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	repo := filepath.Join(root, "demo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "init")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "README.md")
	gitRun(t, repo, "commit", "-m", "init")
	gitRun(t, repo, "worktree", "add", filepath.Join(repo, "feature"), "-b", "feature")

	if err := config.AddSite(config.Site{Name: "demo", Domains: []string{"demo.test"}, Path: repo}); err != nil {
		t.Fatal("add site:", err)
	}

	result, rpcErr := execWorktreeList(map[string]any{"site": "demo"})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		Site      string `json:"site"`
		Worktrees []struct {
			Branch string `json:"branch"`
			Domain string `json:"domain"`
		} `json:"worktrees"`
	}
	decodeContent(t, result, &parsed)
	if parsed.Site != "demo" {
		t.Errorf("expected site=demo, got %q", parsed.Site)
	}
	if len(parsed.Worktrees) != 1 || parsed.Worktrees[0].Branch != "feature" {
		t.Fatalf("expected one worktree on branch feature, got %+v", parsed.Worktrees)
	}
	if parsed.Worktrees[0].Domain != "feature.demo.test" {
		t.Errorf("expected domain feature.demo.test, got %q", parsed.Worktrees[0].Domain)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// initRepoSite builds a real git repo registered as a lerd site, the fixture
// the worktree actions run against.
func initRepoSite(t *testing.T, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))

	repo := filepath.Join(root, name)
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "init")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "README.md")
	gitRun(t, repo, "commit", "-m", "init")
	if err := config.AddSite(config.Site{Name: name, Domains: []string{name + ".test"}, Path: repo}); err != nil {
		t.Fatal("add site:", err)
	}
	return repo
}

// stubWorktreeWait swaps the wait seam so tests need neither a lerd binary on
// PATH nor a running watcher, and records the path it was asked about. The
// watcher is reported up so the wait is reached whatever the host's systemd
// state, which the not-running case below overrides for itself.
func stubWorktreeWait(t *testing.T, code int) *string {
	t.Helper()
	stubWatcherRunning(t, true)
	var gotPath string
	orig := worktreeWaitFn
	t.Cleanup(func() { worktreeWaitFn = orig })
	worktreeWaitFn = func(path string, _ time.Duration) (int, string) {
		gotPath = path
		return code, ""
	}
	return &gotPath
}

func stubWatcherRunning(t *testing.T, running bool) {
	t.Helper()
	orig := watcherRunningFn
	t.Cleanup(func() { watcherRunningFn = orig })
	watcherRunningFn = func() bool { return running }
}

func TestWorktreeTool_advertisesWait(t *testing.T) {
	enum := worktreeTool().InputSchema.Properties["action"].Enum
	for _, a := range enum {
		if a == "wait" {
			return
		}
	}
	t.Errorf("action enum %v must offer wait, or no assistant can discover it", enum)
}

// The race this closes: the watcher starts installing the moment git writes the
// worktree entry, so an add that returns immediately hands back a tree being
// written underneath the caller.
func TestExecWorktreeAdd_waitsForThePipeline(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gotPath := stubWorktreeWait(t, 0)

	result, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
	})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Path        string `json:"path"`
		Provisioned bool   `json:"provisioned"`
	}
	decodeContent(t, result, &parsed)
	if !parsed.OK || !parsed.Provisioned {
		t.Errorf("ok=%v provisioned=%v, want both true", parsed.OK, parsed.Provisioned)
	}
	if *gotPath == "" {
		t.Fatal("add returned without waiting for the pipeline")
	}
	if filepath.Base(*gotPath) != "feature" {
		t.Errorf("waited on %q, want the newly created worktree", *gotPath)
	}
	if parsed.Path == "" {
		t.Error("response must report the new worktree path")
	}
}

// A pipeline still running is not an error, the tree just is not safe to touch
// yet, so the caller is told rather than handed a failure.
func TestExecWorktreeAdd_reportsAnUnfinishedPipeline(t *testing.T) {
	repo := initRepoSite(t, "demo")
	stubWorktreeWait(t, 1)

	result, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
	})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Provisioned bool   `json:"provisioned"`
		Note        string `json:"note"`
	}
	decodeContent(t, result, &parsed)
	if !parsed.OK {
		t.Error("git succeeded, so ok must stay true")
	}
	if parsed.Provisioned {
		t.Error("provisioned must be false while setup is still running")
	}
	if parsed.Note == "" {
		t.Error("an unfinished pipeline needs a note telling the caller to wait")
	}
}

func TestExecWorktreeAdd_waitCanBeDeclined(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gotPath := stubWorktreeWait(t, 0)

	if _, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
		"wait":     false,
	}); rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	if *gotPath != "" {
		t.Errorf("wait=false must not block, but it waited on %q", *gotPath)
	}
}

func TestExecWorktreeWait_reportsSettled(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gitRun(t, repo, "worktree", "add", filepath.Join(repo, "feature"), "-b", "feature")
	stubWorktreeWait(t, 0)

	result, rpcErr := execWorktreeWait(map[string]any{"site": "demo", "branch": "feature"})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		Provisioned bool `json:"provisioned"`
	}
	decodeContent(t, result, &parsed)
	if !parsed.Provisioned {
		t.Error("provisioned must be true when the wait exits 0")
	}
}

func TestExecWorktreeWait_timeoutIsNotSuccess(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gitRun(t, repo, "worktree", "add", filepath.Join(repo, "feature"), "-b", "feature")
	stubWorktreeWait(t, 1)

	result, rpcErr := execWorktreeWait(map[string]any{"site": "demo", "branch": "feature"})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		Provisioned bool `json:"provisioned"`
	}
	decodeContent(t, result, &parsed)
	if parsed.Provisioned {
		t.Error("a timeout must not be reported as provisioned")
	}
}

// Nothing provisions a worktree while the watcher is down, so waiting would
// burn the full timeout before reporting the same failure.
func TestExecWorktreeAdd_doesNotWaitWhenTheWatcherIsDown(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gotPath := stubWorktreeWait(t, 0)
	stubWatcherRunning(t, false)

	result, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
	})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	if *gotPath != "" {
		t.Errorf("waited on %q with no watcher to finish the work", *gotPath)
	}
	var parsed struct {
		Provisioned bool   `json:"provisioned"`
		Note        string `json:"note"`
	}
	decodeContent(t, result, &parsed)
	if parsed.Provisioned {
		t.Error("provisioned must be false when the watcher never ran")
	}
	if !strings.Contains(parsed.Note, "lerd-watcher") {
		t.Errorf("note %q must name the watcher so the caller can fix it", parsed.Note)
	}
}
