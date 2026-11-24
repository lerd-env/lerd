package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// worktreeWorkerEnvSite is a linked site with one git worktree, on a framework
// whose worker_env names queue, returning the parent and worktree paths.
func worktreeWorkerEnvSite(t *testing.T) (string, string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "lerd", "frameworks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveFramework(&config.Framework{
		Name: "testfw",
		Workers: map[string]config.FrameworkWorker{
			"queue": {Command: "php artisan queue:work"},
			"vite":  {Command: "npm run dev", Host: true},
		},
		Worktree: &config.FrameworkWorktree{WorkerEnv: &config.FrameworkWorktreeWorkerEnv{
			Workers: []string{"queue"},
			Env:     map[string]string{"REDIS_PREFIX": "{{parent}}_{{branch}}_database_"},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	dir := gitRepo(t, ".env\n")
	for _, args := range [][]string{
		{"add", ".gitignore"},
		{"commit", "-qm", "init"},
		{"worktree", "add", "-q", "-b", "feat", filepath.Join(dir, "myapp-feat")},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	wt := filepath.Join(dir, "myapp-feat")
	for _, p := range []string{dir, wt} {
		if err := os.WriteFile(filepath.Join(p, ".env"), []byte("REDIS_HOST=lerd-redis\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.AddSite(config.Site{Name: "myapp", Path: dir, Domains: []string{"myapp.test"}, Framework: "testfw"}); err != nil {
		t.Fatal(err)
	}
	return dir, wt
}

func redisPrefix(t *testing.T, dir string) string {
	t.Helper()
	return envfile.Reader(filepath.Join(dir, ".env"), "dotenv")("REDIS_PREFIX")
}

// Starting a worktree's queue gives the worktree its own queue storage first;
// a worker the definition doesn't name, and the parent's own queue, leave the
// env alone, so a worktree without its own queue keeps feeding the parent's.
func TestApplyWorktreeWorkerEnv_onlyForNamedWorkersInAWorktree(t *testing.T) {
	dir, wt := worktreeWorkerEnvSite(t)

	if err := applyWorktreeWorkerEnv("myapp", wt, "vite"); err != nil {
		t.Fatal(err)
	}
	if got := redisPrefix(t, wt); got != "" {
		t.Errorf("vite start wrote REDIS_PREFIX=%q", got)
	}

	if err := applyWorktreeWorkerEnv("myapp", dir, "queue"); err != nil {
		t.Fatal(err)
	}
	if got := redisPrefix(t, dir); got != "" {
		t.Errorf("the parent's queue wrote REDIS_PREFIX=%q into the parent", got)
	}

	if err := applyWorktreeWorkerEnv("myapp", wt, "queue"); err != nil {
		t.Fatal(err)
	}
	if got := redisPrefix(t, wt); got != "myapp_feat_database_" {
		t.Errorf("worktree REDIS_PREFIX = %q, want myapp_feat_database_", got)
	}
}

// Starting a proxy worker in a worktree is refused with the reason, rather
// than starting a second process on the parent's port.
func TestWorktreeWorkerBlock_refusesProxyWorkersInAWorktree(t *testing.T) {
	dir, wt := worktreeWorkerEnvSite(t)
	reverb := config.FrameworkWorker{Command: "php artisan reverb:start", Proxy: &config.WorkerProxy{Path: "/app"}}

	if err := worktreeWorkerBlock("myapp", dir, "reverb", reverb); err != nil {
		t.Errorf("the parent's reverb was refused: %v", err)
	}
	if err := worktreeWorkerBlock("myapp", wt, "reverb", reverb); err == nil {
		t.Error("a worktree reverb was allowed to start on the parent's port")
	}
	if err := worktreeWorkerBlock("myapp", wt, "queue", config.FrameworkWorker{Command: "x"}); err != nil {
		t.Errorf("a worktree queue was refused: %v", err)
	}
}
