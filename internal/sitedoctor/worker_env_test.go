package sitedoctor

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// horizonFramework is a definition shaped like the store's: a worker gated on a
// package plus an env value, and a service whose detect rules claim that value.
func horizonFramework() *config.Framework {
	return &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"horizon": {Command: "php artisan horizon", Check: &config.FrameworkRule{Composer: "laravel/horizon", WhenEnv: "QUEUE_CONNECTION=redis"}},
		},
		Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
			"redis": {Detect: []config.FrameworkServiceDetect{{Key: "REDIS_HOST"}, {Key: "QUEUE_CONNECTION", ValuePrefix: "redis"}}},
		}},
	}
}

func writeHorizonSite(t *testing.T, lerdYAML, env string) string {
	t.Helper()
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", lerdYAML)
	writeEnv(t, dir, ".env", env)
	writeEnv(t, dir, "composer.json", `{"require":{"laravel/horizon":"^5.0"}}`)
	return dir
}

func TestCheckWorkerEnv_OffersFixWhenServiceDeclared(t *testing.T) {
	dir := writeHorizonSite(t, "services:\n  - redis\nworkers:\n  - horizon\n", "QUEUE_CONNECTION=sync\n")
	c, ok := checkWorkerEnv(dir, horizonFramework())
	if !ok || c.Status != StatusWarn || c.Fix != FixWorkerEnv {
		t.Fatalf("got ok=%v %+v, want a fixable warning", ok, c)
	}
	if want := "horizon needs QUEUE_CONNECTION=redis (currently sync), lerd skips it"; c.Detail != want {
		t.Errorf("detail %q, want %q", c.Detail, want)
	}
}

// Without the service the value points at, switching the env would only trade
// one broken worker for another, so the gap is reported but not fixed.
func TestCheckWorkerEnv_NoFixWithoutService(t *testing.T) {
	dir := writeHorizonSite(t, "workers:\n  - horizon\n", "")
	c, ok := checkWorkerEnv(dir, horizonFramework())
	if !ok || c.Fix != "" {
		t.Fatalf("got ok=%v %+v, want a warning with no fix", ok, c)
	}
	if want := "horizon needs QUEUE_CONNECTION=redis (currently unset), lerd skips it"; c.Detail != want {
		t.Errorf("detail %q, want %q", c.Detail, want)
	}
}

func TestCheckWorkerEnv_AbsentWhenGatePasses(t *testing.T) {
	dir := writeHorizonSite(t, "services:\n  - redis\nworkers:\n  - horizon\n", "QUEUE_CONNECTION=redis\n")
	if c, ok := checkWorkerEnv(dir, horizonFramework()); ok {
		t.Errorf("got %+v, want no check", c)
	}
}

// A missing package is project_config's finding: setting the env would not
// make the worker runnable.
func TestCheckWorkerEnv_AbsentWhenPackageMissing(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", "services:\n  - redis\nworkers:\n  - horizon\n")
	writeEnv(t, dir, ".env", "QUEUE_CONNECTION=sync\n")
	if c, ok := checkWorkerEnv(dir, horizonFramework()); ok {
		t.Errorf("got %+v, want no check", c)
	}
}
