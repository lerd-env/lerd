package siteinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func vitePerWT() config.FrameworkWorker {
	tr := true
	return config.FrameworkWorker{Label: "Vite", Command: "npm run dev", PerWorktree: &tr}
}

func TestEnrichWorktreeWorkers_listsEveryWorkerThatApplies(t *testing.T) {
	// A worktree on its own Redis needs its own queue consumer, so every worker
	// that applies at the worktree is offered there, stopped until started.
	origUnit := unitStatusFn
	unitStatusFn = func(name string) (string, error) {
		if name == "lerd-queue-rapids-feat" {
			return "active", nil
		}
		return "inactive", nil
	}
	defer func() { unitStatusFn = origUnit }()

	wt := filepath.Join(t.TempDir(), "feat")
	if err := os.Mkdir(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"queue":   {Command: "x", ExcludeCheck: &config.FrameworkRule{File: "horizon.flag"}},
			"horizon": {Command: "x", Check: &config.FrameworkRule{File: "horizon.flag"}},
			"vite":    vitePerWT(),
		},
	}
	got := enrichWorktreeWorkers("rapids", wt, fw)
	if len(got) != 2 || got[0].Name != "queue" || got[1].Name != "vite" {
		t.Fatalf("expected queue and vite, got %+v", got)
	}
	if !got[0].Running || got[1].Running {
		t.Errorf("only the worktree's own queue unit is running, got %+v", got)
	}

	if err := os.WriteFile(filepath.Join(wt, "horizon.flag"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got = enrichWorktreeWorkers("rapids", wt, fw)
	if len(got) != 2 || got[0].Name != "horizon" || got[1].Name != "vite" {
		t.Fatalf("horizon should replace queue once it applies, got %+v", got)
	}
}

func TestEnrichWorktreeWorkers_unitNamePerWorktree(t *testing.T) {
	origUnit := unitStatusFn
	unitStatusFn = func(name string) (string, error) {
		if name == "lerd-vite-rapids-main" {
			return "active", nil
		}
		return "inactive", nil
	}
	defer func() { unitStatusFn = origUnit }()

	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"vite": vitePerWT(),
		},
	}
	got := enrichWorktreeWorkers("rapids", "/projects/rapids/main", fw)
	if len(got) != 1 {
		t.Fatalf("expected 1 worker, got %d", len(got))
	}
	if got[0].Name != "vite" || !got[0].Running || got[0].Failing {
		t.Errorf("vite worker not reported running: %+v", got[0])
	}
	if got[0].Label != "Vite" {
		t.Errorf("label = %q, want Vite", got[0].Label)
	}
}

func TestEnrichWorktreeWorkers_inactiveOmitted(t *testing.T) {
	origUnit := unitStatusFn
	unitStatusFn = func(string) (string, error) { return "inactive", nil }
	defer func() { unitStatusFn = origUnit }()

	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"vite": vitePerWT(),
		},
	}
	got := enrichWorktreeWorkers("site", "/wt/path", fw)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Running || got[0].Failing {
		t.Errorf("expected inactive flags, got %+v", got[0])
	}
}

func TestEnrichWorktreeWorkers_failedReported(t *testing.T) {
	origUnit := unitStatusFn
	unitStatusFn = func(string) (string, error) { return "failed", nil }
	defer func() { unitStatusFn = origUnit }()

	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"vite": vitePerWT(),
		},
	}
	got := enrichWorktreeWorkers("site", "/wt/path", fw)
	if len(got) != 1 || got[0].Running || !got[0].Failing {
		t.Errorf("expected failing flag, got %+v", got)
	}
}

// A worker served through a proxy (Reverb) needs its own port and route on the
// worktree's domain, which only per_worktree workers get, so it stays on main.
func TestEnrichWorktreeWorkers_keepsProxyWorkersOnMain(t *testing.T) {
	origUnit := unitStatusFn
	unitStatusFn = func(string) (string, error) { return "inactive", nil }
	defer func() { unitStatusFn = origUnit }()

	fw := &config.Framework{
		Workers: map[string]config.FrameworkWorker{
			"queue":  {Command: "x"},
			"reverb": {Command: "x", Proxy: &config.WorkerProxy{Path: "/app"}},
		},
	}
	got := enrichWorktreeWorkers("site", t.TempDir(), fw)
	if len(got) != 1 || got[0].Name != "queue" {
		t.Errorf("expected only queue, got %+v", got)
	}
}
