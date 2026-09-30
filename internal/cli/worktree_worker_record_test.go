package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// worktreeOfSite makes a git repo with a committed .lerd.yaml, registers it as
// a site and checks out a worktree of it, returning the worktree's path.
func worktreeOfSite(t *testing.T) (site, wt string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	site = gitRepo(t, "")
	if err := os.WriteFile(filepath.Join(site, ".lerd.yaml"), []byte("workers:\n- queue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt = filepath.Join(site, filepath.Base(site)+"-feat")
	for _, args := range [][]string{
		{"add", ".lerd.yaml"},
		{"commit", "-q", "-m", "init"},
		{"worktree", "add", "-q", "-b", "feat", wt},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = site
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, Domains: []string{"app.test"}}); err != nil {
		t.Fatal(err)
	}
	return site, wt
}

func gitStatus(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v (%s)", err, out)
	}
	return strings.TrimSpace(string(out))
}

// A worker started in a worktree belongs to that checkout: recording it in the
// committed .lerd.yaml left the tree modified, so git refused to remove it.
func TestRecordProjectWorker_WorktreeKeepsItOutOfTheCommittedFile(t *testing.T) {
	_, wt := worktreeOfSite(t)

	recordProjectWorker(wt, "vite")

	if got := gitStatus(t, wt); got != "" {
		t.Errorf("worktree left dirty after starting a worker:\n%s", got)
	}
	cfg, err := config.LoadProjectConfig(wt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Workers, ",") != "queue,vite" {
		t.Errorf("workers = %v, want the committed queue plus vite", cfg.Workers)
	}
}

func TestRecordProjectWorker_SiteStillRecordsInLerdYAML(t *testing.T) {
	site, _ := worktreeOfSite(t)

	recordProjectWorker(site, "vite")

	data, err := os.ReadFile(filepath.Join(site, ".lerd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "vite") {
		t.Errorf(".lerd.yaml = %q, want vite recorded for the site itself", data)
	}
	if _, err := os.Stat(filepath.Join(site, config.LocalOverrideFile)); err == nil {
		t.Error("the site itself got a .lerd.local.yaml; only worktrees record there")
	}
}

// Without a terminal there is nobody to ask whether to force the removal, so it
// has to fail with a message naming --force rather than a raw tty error.
func TestRunGitWorktreeRemove_NoTerminalNamesForce(t *testing.T) {
	site, wt := worktreeOfSite(t)
	if err := os.WriteFile(filepath.Join(wt, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(site)

	err := runGitWorktreeRemove([]string{wt})

	if err == nil {
		t.Fatal("removing a modified worktree without --force succeeded")
	}
	if !strings.Contains(err.Error(), "--force") || strings.Contains(err.Error(), "tty") {
		t.Errorf("error = %q, want a hint to rerun with --force and no tty error", err)
	}
	if _, statErr := os.Stat(wt); statErr != nil {
		t.Error("the worktree was removed although nobody confirmed it")
	}
}
