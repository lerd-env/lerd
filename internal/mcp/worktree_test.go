package mcp

import (
	"errors"
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
// PATH nor a running watcher, and records the path it was asked about. Setup is
// stubbed too, since the real one shells out to the running binary. The
// watcher is reported up so the wait is reached whatever the host's systemd
// state, which the not-running case below overrides for itself.
func stubWorktreeWait(t *testing.T, code int) *string {
	t.Helper()
	stubWatcherRunning(t, true)
	stubWorktreeSetup(t, nil)
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

type worktreeSetupCall struct{ path, build, db string }

func stubWorktreeSetup(t *testing.T, err error) *[]worktreeSetupCall {
	t.Helper()
	var calls []worktreeSetupCall
	orig := worktreeSetupFn
	t.Cleanup(func() { worktreeSetupFn = orig })
	worktreeSetupFn = func(path, build, db string) (string, error) {
		calls = append(calls, worktreeSetupCall{path, build, db})
		return "", err
	}
	return &calls
}

// Deps alone leave a tree that fails its first request: no asset build and no
// database wiring. add finishes the setup the way the dashboard's add does.
func TestExecWorktreeAdd_finishesSetupOnceProvisioned(t *testing.T) {
	repo := initRepoSite(t, "demo")
	stubWorktreeWait(t, 0)
	calls := stubWorktreeSetup(t, nil)

	result, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
	})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		Ready bool `json:"ready"`
	}
	decodeContent(t, result, &parsed)
	if len(*calls) != 1 {
		t.Fatalf("setup ran %d times, want once", len(*calls))
	}
	got := (*calls)[0]
	if filepath.Base(got.path) != "feature" || got.build != "auto" || got.db != "" {
		t.Errorf("setup called with %+v, want the new worktree, build auto, no db request", got)
	}
	if !parsed.Ready {
		t.Error("a finished setup must report ready")
	}
}

func TestExecWorktreeAdd_passesBuildAndDBChoices(t *testing.T) {
	repo := initRepoSite(t, "demo")
	stubWorktreeWait(t, 0)
	calls := stubWorktreeSetup(t, nil)

	if _, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
		"build":    "skip",
		"db":       "clone-main",
	}); rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	if len(*calls) != 1 || (*calls)[0].build != "skip" || (*calls)[0].db != "clone-main" {
		t.Errorf("setup calls %+v, want build skip and db clone-main forwarded", *calls)
	}
}

// Building into a tree still being installed is the race wait exists to stop.
func TestExecWorktreeAdd_skipsSetupUntilProvisioned(t *testing.T) {
	repo := initRepoSite(t, "demo")
	stubWorktreeWait(t, 1)
	calls := stubWorktreeSetup(t, nil)

	if _, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
	}); rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	if len(*calls) != 0 {
		t.Errorf("setup ran on an unprovisioned tree: %+v", *calls)
	}
}

func TestExecWorktreeAdd_reportsAFailedSetup(t *testing.T) {
	repo := initRepoSite(t, "demo")
	stubWorktreeWait(t, 0)
	stubWorktreeSetup(t, errors.New("exit status 1"))

	result, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{filepath.Join(repo, "feature"), "-b", "feature"},
	})
	if rpcErr != nil {
		t.Fatal("unexpected rpc error:", rpcErr.Message)
	}
	var parsed struct {
		OK    bool   `json:"ok"`
		Ready bool   `json:"ready"`
		Note  string `json:"note"`
	}
	decodeContent(t, result, &parsed)
	if !parsed.OK || parsed.Ready || parsed.Note == "" {
		t.Errorf("ok=%v ready=%v note=%q, want ok, not ready, and a note", parsed.OK, parsed.Ready, parsed.Note)
	}
}

// git refuses `worktree add -b <branch>` without a path, so add fills in the
// same checkout path the CLI and the dashboard use.
func TestExecWorktreeAdd_derivesThePathForANewBranch(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gotPath := stubWorktreeWait(t, 0)

	if result, rpcErr := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"git_args": []any{"-b", "feat-x"},
	}); rpcErr != nil || result.(map[string]any)["isError"] == true {
		t.Fatalf("add failed: %v %v", rpcErr, result)
	}
	if want := filepath.Join(repo, filepath.Base(repo)+"-feat-x"); *gotPath != want {
		t.Errorf("worktree at %q, want %q", *gotPath, want)
	}
}

// branch names a branch that may not exist yet; asking for a worktree on it is
// asking for the branch too.
func TestExecWorktreeAdd_branchCreatesAMissingBranch(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gotPath := stubWorktreeWait(t, 0)

	if result, rpcErr := execWorktreeAdd(map[string]any{
		"site":   "demo",
		"branch": "feat-y",
	}); rpcErr != nil || result.(map[string]any)["isError"] == true {
		t.Fatalf("add failed: %v %v", rpcErr, result)
	}
	if want := filepath.Join(repo, filepath.Base(repo)+"-feat-y"); *gotPath != want {
		t.Errorf("worktree at %q, want %q", *gotPath, want)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// Passing a start point as git_args beside branch used to drop the branch and
// leave a detached checkout named after the start point.
func TestExecWorktreeAdd_baseStartsTheNewBranchFromIt(t *testing.T) {
	repo := initRepoSite(t, "demo")
	gitRun(t, repo, "branch", "release")
	gitRun(t, repo, "checkout", "-q", "release")
	gitRun(t, repo, "commit", "-q", "--allow-empty", "-m", "ahead")
	gitRun(t, repo, "checkout", "-q", "-")
	stubWorktreeWait(t, 0)

	if result, rpcErr := execWorktreeAdd(map[string]any{
		"site":   "demo",
		"branch": "explore",
		"base":   "release",
	}); rpcErr != nil || result.(map[string]any)["isError"] == true {
		t.Fatalf("add failed: %v %v", rpcErr, result)
	}
	wt := filepath.Join(repo, filepath.Base(repo)+"-explore")
	if got := gitOut(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); got != "explore" {
		t.Errorf("worktree is on %q, want the new branch explore", got)
	}
	if gitOut(t, wt, "rev-parse", "HEAD") != gitOut(t, repo, "rev-parse", "release") {
		t.Error("explore must start from base")
	}
}

func TestExecWorktreeAdd_refusesBranchWithGitArgs(t *testing.T) {
	initRepoSite(t, "demo")
	stubWorktreeWait(t, 0)

	result, _ := execWorktreeAdd(map[string]any{
		"site":     "demo",
		"branch":   "explore",
		"git_args": []any{"-b", "other"},
	})
	if result.(map[string]any)["isError"] != true {
		t.Error("branch beside git_args must be refused, not silently dropped")
	}
}

// list names a detached checkout detached-<sha>, which is no path git knows.
func TestExecWorktreeRemove_removesADetachedWorktree(t *testing.T) {
	repo := initRepoSite(t, "demo")
	wt := filepath.Join(repo, "demo-detached")
	gitRun(t, repo, "worktree", "add", "-q", "--detach", wt)
	branch := "detached-" + gitOut(t, repo, "rev-parse", "--short=7", "HEAD")

	if result, rpcErr := execWorktreeRemove(map[string]any{"site": "demo", "branch": branch}); rpcErr != nil || result.(map[string]any)["isError"] == true {
		t.Fatalf("remove failed: %v %v", rpcErr, result)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("the detached worktree is still on disk")
	}
}
