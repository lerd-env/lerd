package sitedoctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func statusByName(resp Response) map[string]string {
	out := map[string]string{}
	for _, c := range resp.Checks {
		out[c.Name] = c.Status
	}
	return out
}

// The quick pass is what `lerd doctor` sweeps every site with, so it must leave
// out everything that shells into the site: the framework command checks and the
// composer audit. What it keeps is the file-level dependency state.
func TestRunWithQuick_LeavesOutTheContainerChecks(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_KEY=base64:key\n")
	writeEnv(t, dir, "composer.json", "{}")
	writeEnv(t, dir, "composer.lock", "{}")
	mustMkdir(t, filepath.Join(dir, "vendor"))
	fw := &config.Framework{
		Name: "laravel",
		Env:  config.FrameworkEnvConf{File: ".env"},
		Doctor: &config.FrameworkDoctor{Checks: []config.DoctorCheck{
			{Name: "migrations", Type: "command", Command: "echo Pending", FailIfOutputContains: "Pending"},
		}},
	}

	quick := statusByName(RunWith(context.Background(), dir, fw, Options{Quick: true}))
	if _, ran := quick["migrations"]; ran {
		t.Error("quick must skip the framework command checks")
	}
	if _, ran := quick["composer_audit"]; ran {
		t.Error("quick must skip the composer audit")
	}
	if quick["composer_deps"] != StatusOK {
		t.Errorf("composer_deps: got %q, want the file-level check to still run", quick["composer_deps"])
	}
}

// The full report keeps running the command checks the quick one drops.
func TestRunWithFull_StillRunsTheCommandChecks(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".env", "APP_KEY=base64:key\n")
	fw := &config.Framework{
		Name: "laravel",
		Env:  config.FrameworkEnvConf{File: ".env"},
		Doctor: &config.FrameworkDoctor{Checks: []config.DoctorCheck{
			{Name: "migrations", Type: "command", Command: "echo Pending", FailIfOutputContains: "Pending"},
		}},
	}
	if got := statusByName(Run(context.Background(), dir, fw))["migrations"]; got != StatusFail {
		t.Errorf("migrations: got %q, want the command check to run and fail", got)
	}
}

// A missing vendor directory is the one composer finding the quick pass can
// still make on its own.
func TestQuickComposerDeps_FlagsMissingVendor(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, "composer.json", "{}")
	tasks := dependencyCheckTasks(context.Background(), dir, nil, Options{Quick: true})
	if len(tasks) != 1 {
		t.Fatalf("want 1 quick composer task, got %d", len(tasks))
	}
	c, _ := tasks[0]()
	if c.Status != StatusWarn || c.Fix != FixComposerInstall {
		t.Errorf("got status=%q fix=%q, want warn/%s", c.Status, c.Fix, FixComposerInstall)
	}
}
