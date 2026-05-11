//go:build darwin

package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestExpectedExecWorkers_filtersUnsupportedShapes pins the
// platform-gate filter so the watcher doesn't burn cooldown windows
// trying to heal worker shapes the macOS path can't run. The darwin
// gate rejects Schedule != "" (launchd StartCalendarInterval isn't
// wired through services.Mgr yet); host:true is now supported.
//
// Without the filter the heal loop logs "self-healing exec-mode
// worker" + the WARN from WorkerSupportedOnPlatform every 2 minutes
// for the same unsupported unit.
func TestExpectedExecWorkers_filtersUnsupportedShapes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	sitePath := filepath.Join(tmp, "acme")
	if err := os.MkdirAll(sitePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sitePath, ".lerd.yaml"), []byte(
		"framework: laravel\nworkers:\n  - vite\n  - schedule\n  - queue\n",
	), 0644); err != nil {
		t.Fatal(err)
	}

	if err := config.AddSite(config.Site{
		Name: "acme", Domains: []string{"acme.test"},
		Path: sitePath, Framework: "laravel", PHPVersion: "8.4",
	}); err != nil {
		t.Fatal(err)
	}

	// Inject synthetic worker definitions via the project's
	// CustomWorkers so the test doesn't depend on the live laravel
	// framework yaml. tr is a pointer for the *bool PerWorktree field.
	proj, _ := config.LoadProjectConfig(sitePath)
	tr := true
	proj.CustomWorkers = map[string]config.FrameworkWorker{
		"vite":     {Command: "npm run dev", Host: true, PerWorktree: &tr},
		"schedule": {Command: "php artisan schedule:run", Schedule: "minutely"},
		"queue":    {Command: "php artisan queue:work"},
	}
	if err := config.SaveProjectConfig(sitePath, proj); err != nil {
		t.Fatal(err)
	}

	expected := expectedExecWorkers()

	hasUnit := func(unit string) bool {
		for _, w := range expected {
			if w.unit == unit {
				return true
			}
		}
		return false
	}
	if !hasUnit("lerd-vite-acme") {
		t.Errorf("expected vite (host:true, supported) to be enumerated; got %+v", units(expected))
	}
	if !hasUnit("lerd-queue-acme") {
		t.Errorf("expected queue (plain exec-mode worker) to be enumerated; got %+v", units(expected))
	}
	if hasUnit("lerd-schedule-acme") {
		t.Errorf("expected schedule (Schedule != \"\") to be filtered by WorkerSupportedOnPlatform; got %+v", units(expected))
	}
}

func units(ws []expectedExecWorker) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.unit)
	}
	return out
}
