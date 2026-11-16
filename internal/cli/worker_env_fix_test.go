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
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	if err := config.SaveFramework(&config.Framework{
		Name: "gatedfw",
		Workers: map[string]config.FrameworkWorker{
			"horizon": {Command: "php artisan horizon", Check: &config.FrameworkRule{Composer: "laravel/horizon", WhenEnv: "QUEUE_CONNECTION=redis"}},
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
	workers, err := ApplyWorkerEnv(site, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) != 1 || workers[0] != "horizon" {
		t.Errorf("workers = %v, want [horizon]", workers)
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
	workers, err := ApplyWorkerEnv(site, dir)
	if err != nil || workers != nil {
		t.Fatalf("got workers=%v err=%v, want nothing done", workers, err)
	}
	if env := readEnv(t, dir); !strings.Contains(env, "QUEUE_CONNECTION=sync") {
		t.Errorf(".env = %q, want it untouched", env)
	}
}
