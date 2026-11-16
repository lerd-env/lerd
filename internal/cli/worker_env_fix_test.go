package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// setupWorkerEnvSite saves a framework with an env-gated worker and returns a
// checkout that has the package but the wrong .env value. The site points
// elsewhere, so the checkout is a worktree and no worker is started.
func setupWorkerEnvSite(t *testing.T, lerdYAML string) (config.Site, string) {
	t.Helper()
	return setupWorkerEnvSiteWith(t, lerdYAML, nil)
}

func setupWorkerEnvSiteWith(t *testing.T, lerdYAML string, perWorktree *bool) (config.Site, string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	if err := config.SaveFramework(&config.Framework{
		Name: "gatedfw",
		Workers: map[string]config.FrameworkWorker{
			"horizon": {Command: "php artisan horizon", PerWorktree: perWorktree, Check: &config.FrameworkRule{Composer: "laravel/horizon", WhenEnv: "QUEUE_CONNECTION=redis"}},
		},
		Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
			"redis": {Detect: []config.FrameworkServiceDetect{{Key: "QUEUE_CONNECTION", ValuePrefix: "redis"}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		".lerd.yaml":    lerdYAML,
		".env":          "APP_NAME=demo\nQUEUE_CONNECTION=sync\n",
		"composer.json": `{"require":{"laravel/horizon":"^5.0"}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return config.Site{Name: "demo", Path: t.TempDir(), Framework: "gatedfw"}, dir
}

func readEnv(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyWorkerEnv_WritesTheGatedValue(t *testing.T) {
	site, dir := setupWorkerEnvSite(t, "services:\n  - redis\nworkers:\n  - horizon\n")
	done, err := ApplyWorkerEnv(site, dir)
	if err != nil {
		t.Fatal(err)
	}
	// horizon is not per_worktree, so nothing runs it for this worktree.
	if done != "set the .env value" {
		t.Errorf("done = %q, want only the .env write reported", done)
	}
	env := readEnv(t, dir)
	if !strings.Contains(env, "QUEUE_CONNECTION=redis") || strings.Contains(env, "QUEUE_CONNECTION=sync") {
		t.Errorf(".env = %q, want QUEUE_CONNECTION switched to redis", env)
	}
	if !strings.Contains(env, "APP_NAME=demo") {
		t.Errorf(".env = %q, lost the other keys", env)
	}
}

// Without the service the value points at, the fix leaves .env alone.
func TestApplyWorkerEnv_SkipsGapWithoutService(t *testing.T) {
	site, dir := setupWorkerEnvSite(t, "workers:\n  - horizon\n")
	done, err := ApplyWorkerEnv(site, dir)
	if err != nil || done != "" {
		t.Fatalf("got done=%q err=%v, want nothing done", done, err)
	}
	if env := readEnv(t, dir); !strings.Contains(env, "QUEUE_CONNECTION=sync") {
		t.Errorf(".env = %q, want it untouched", env)
	}
}

// A per_worktree worker gated on the value is started for the worktree itself,
// on the worktree's own PHP, since nothing else starts it once .env changes.
func TestApplyWorkerEnv_StartsPerWorktreeWorkerInTheWorktree(t *testing.T) {
	yes := true
	site, dir := setupWorkerEnvSiteWith(t, "php_version: \"8.3\"\nservices:\n  - redis\nworkers:\n  - horizon\n", &yes)
	site.PHPVersion = "8.5"
	var startedAt []string
	orig := startGatedWorker
	startGatedWorker = func(_ config.Site, path, php, name string, _ config.FrameworkWorker) error {
		startedAt = append(startedAt, name+"@"+path+" php"+php)
		return nil
	}
	t.Cleanup(func() { startGatedWorker = orig })

	done, err := ApplyWorkerEnv(site, dir)
	if err != nil {
		t.Fatal(err)
	}
	if done != "set the .env value and started horizon" {
		t.Errorf("done = %q", done)
	}
	if len(startedAt) != 1 || startedAt[0] != "horizon@"+dir+" php8.3" {
		t.Errorf("started %v, want horizon at the worktree path on its PHP 8.3", startedAt)
	}
}

// Run from a worktree, the CLI fix finds the parent site through git's worktree
// metadata, since the registry only lists the parent's own folder.
func TestSiteForCheckout_ResolvesWorktreeToParent(t *testing.T) {
	site, dir := setupWorkerEnvSite(t, "services:\n  - redis\nworkers:\n  - horizon\n")
	meta := filepath.Join(site.Path, ".git", "worktrees", "feat")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "gitdir"), []byte(filepath.Join(dir, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}

	got, ok := siteForCheckout(dir)
	if !ok || got.Name != site.Name {
		t.Fatalf("got %+v ok=%v, want the parent site %q", got, ok, site.Name)
	}
	if _, ok := siteForCheckout(t.TempDir()); ok {
		t.Error("an unrelated folder resolved to a site")
	}
}
