package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

func TestWorktreeWorkerIdleSuspended(t *testing.T) {
	site := &config.Site{
		Name: "shop",
		WorktreeIdleSuspended: map[string][]string{
			"feature-x": {"vite", "queue"},
		},
	}
	cases := []struct {
		name   string
		wtPath string
		worker string
		want   bool
	}{
		{"suspended worker in worktree", "/home/u/shop-worktrees/feature-x", "vite", true},
		{"other worker in same worktree", "/home/u/shop-worktrees/feature-x", "reverb", false},
		{"worker in a worktree with no suspensions", "/home/u/shop-worktrees/main", "vite", false},
	}
	for _, c := range cases {
		if got := worktreeWorkerIdleSuspended(site, c.wtPath, c.worker); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIdleTimingStatus(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	timeout := time.Hour
	cases := []struct {
		name      string
		lastUnix  int64
		suspended []string
		want      string
	}{
		{"suspended reads idle", now.Add(-5 * time.Minute).Unix(), []string{"vite"}, "idle 5m"},
		{"recent reads active", now.Add(-5 * time.Minute).Unix(), nil, "active 5m ago"},
		{"past timeout reads idle", now.Add(-2 * time.Hour).Unix(), nil, "idle 2h"},
		{"no record", 0, nil, "no activity yet"},
	}
	for _, c := range cases {
		if got := idleTimingStatus(c.lastUnix, c.suspended, timeout, now); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// A proxy-only site is exempt from suspension, so its status reads that rather
// than an "idle" it can never act on.
func TestIdleSiteStatus_proxyOnly(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	last := map[string]int64{"nestapp": now.Add(-2 * time.Hour).Unix()}
	proxyOnly := config.Site{Name: "nestapp", HostPort: 3000}
	if got := idleSiteStatus(proxyOnly, last, nil, time.Hour, now); got != "proxy only" {
		t.Errorf("proxy-only site = %q, want proxy only", got)
	}
	supervised := config.Site{Name: "nestapp", HostPort: 3000, HostCommand: "npm run dev"}
	if got := idleSiteStatus(supervised, last, nil, time.Hour, now); got != "idle 2h" {
		t.Errorf("supervised host-proxy site = %q, want idle 2h", got)
	}
}

func TestIdleWorktreeStatus_inheritsSitePauseAndPin(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	wt := idleWtState{Branch: "dev", LastActive: now.Unix()}
	if got := idleWorktreeStatus(config.Site{Paused: true}, wt, nil, time.Hour, now); got != "paused" {
		t.Errorf("paused site worktree = %q, want paused", got)
	}
	if got := idleWorktreeStatus(config.Site{Pinned: true}, wt, nil, time.Hour, now); got != "pinned" {
		t.Errorf("pinned site worktree = %q, want pinned", got)
	}
	if got := idleWorktreeStatus(config.Site{}, wt, nil, time.Hour, now); got != "active 0s ago" {
		t.Errorf("active worktree = %q, want active 0s ago", got)
	}
}

// TestWorktreeWorkersToStart_skipsSuspended guards the fix for a suspended
// worktree worker being resurrected by the boot/onAdded autostart scan: a worker
// the engine recorded under worktree_idle_suspended must be filtered out so it
// stays asleep until activity resumes it through the engine.
func TestWorktreeWorkersToStart_skipsSuspended(t *testing.T) {
	site := &config.Site{
		Name: "rental",
		WorktreeIdleSuspended: map[string][]string{
			"rental-main": {"vite"},
		},
	}
	got := worktreeWorkersToStart(site, "rental-main", []string{"vite", "queue"})
	if len(got) != 1 || got[0] != "queue" {
		t.Errorf("worktreeWorkersToStart = %v, want [queue]", got)
	}

	// A worktree with nothing suspended passes the list through untouched.
	all := worktreeWorkersToStart(site, "other", []string{"vite", "queue"})
	if len(all) != 2 {
		t.Errorf("unsuspended worktree = %v, want both workers", all)
	}
}

// TestEnsureViteSleepable_buildsAtMostOnce guards against the engine re-running
// `npm run build` on every tick for a checkout whose build never produces a
// manifest (a common worktree case), which would otherwise thrash CPU forever.
func TestEnsureViteSleepable_buildsAtMostOnce(t *testing.T) {
	dir := t.TempDir() // no built manifest here, so the build is "attempted"
	site := &config.Site{Name: "x", Path: dir}

	var builds int
	prev := runViteBuildAt
	runViteBuildAt = func(*config.Site, string) { builds++ } // never writes a manifest
	t.Cleanup(func() { runViteBuildAt = prev })

	if ensureViteSleepableAt(site, dir) {
		t.Fatal("expected not sleepable without a manifest")
	}
	ensureViteSleepableAt(site, dir) // second tick must not rebuild
	if builds != 1 {
		t.Errorf("build ran %d times, want 1 (memoized after first attempt)", builds)
	}
}

// TestWorktreeIdleSuspendStateIsStale covers the conservative guards: an empty
// persisted set is never stale, and an unknown framework (workers can't be
// enumerated) keeps the worktree suspended rather than wrongly clearing it.
func TestWorktreeIdleSuspendStateIsStale(t *testing.T) {
	site := &config.Site{Name: "myapp", Path: "/srv/myapp", Framework: "laravel"}

	if WorktreeIdleSuspendStateIsStale(site, "feature-x", nil) {
		t.Errorf("empty suspended set reported stale")
	}

	noFW := &config.Site{Name: "myapp", Path: "/srv/myapp", Framework: "__nope__"}
	if WorktreeIdleSuspendStateIsStale(noFW, "feature-x", []string{"vite"}) {
		t.Errorf("unknown framework reported stale; should stay suspended")
	}
}

// TestWorktreeWorkerUnitNaming pins that the unit name collectRunningWorktreeWorkers
// checks (lerd-<w>-<site>-<wtBase>) is exactly what workerNames produces for a
// worktree checkout, so idle-suspend detects, stops, and restarts the same unit.
func TestWorktreeWorkerUnitNaming(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	if err := config.AddSite(config.Site{Name: "myapp", Path: "/srv/myapp", PHPVersion: "8.4"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	// A worktree checkout path differs from the main path, so the unit gets the
	// worktree-base suffix.
	unit, _ := workerNames("myapp", "/srv/myapp/.worktrees/feature-x", "vite")
	if unit != "lerd-vite-myapp-feature-x" {
		t.Errorf("worktree unit = %q, want lerd-vite-myapp-feature-x", unit)
	}

	// The main checkout keeps the plain unit name.
	mainUnit, _ := workerNames("myapp", "/srv/myapp", "vite")
	if mainUnit != "lerd-vite-myapp" {
		t.Errorf("main unit = %q, want lerd-vite-myapp", mainUnit)
	}
}

// A worker switched on for one worktree runs as that worktree's own unit, so it
// sleeps with the worktree and restarts on a database isolation, including one
// only the branch's .lerd.yaml declares. Vite stopped there is not running.
func TestCollectRunningWorktreeWorkers_readsTheWorktreesOwnWorkers(t *testing.T) {
	site, wt := worktreeDefsSite(t)
	fake := &statefulLifecycle{active: map[string]bool{
		"lerd-queue-myapp-feat":  true,
		"lerd-export-myapp-feat": true,
		"lerd-schedule-myapp":    true, // the parent's, not this worktree's
	}}
	podman.UnitLifecycle = fake
	t.Cleanup(func() { podman.UnitLifecycle = nil })

	got := collectRunningWorktreeWorkers(site, wt)
	if len(got) != 2 || got[0] != "export" || got[1] != "queue" {
		t.Errorf("running worktree workers = %v, want [export queue]", got)
	}
}

// A worktree wakes its workers from its own definition, so a worker only the
// branch's .lerd.yaml declares comes back with the branch's command.
func TestWorktreeWorkerDefs_includeTheBranchsCustomWorkers(t *testing.T) {
	site, wt := worktreeDefsSite(t)
	defs := worktreeWorkerDefs(site, wt)
	if defs["export"].Command != "php artisan export:work" {
		t.Errorf("export = %+v, want the branch's own worker", defs["export"])
	}
	if _, ok := defs["queue"]; !ok {
		t.Errorf("queue missing from %v", defs)
	}
}

// worktreeDefsSite is a site on a framework with queue, schedule and a
// per-worktree vite, and a worktree whose .lerd.yaml adds an export worker.
func worktreeDefsSite(t *testing.T) (*config.Site, string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DATA_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "lerd", "frameworks"), 0o755); err != nil {
		t.Fatal(err)
	}
	perWT := true
	if err := config.SaveFramework(&config.Framework{
		Name: "testfw",
		Workers: map[string]config.FrameworkWorker{
			"queue":    {Command: "php artisan queue:work"},
			"schedule": {Command: "php artisan schedule:work"},
			"vite":     {Command: "npm run dev", Host: true, PerWorktree: &perWT},
		},
	}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "feat")
	if err := os.Mkdir(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".lerd.yaml"), []byte("custom_workers:\n  export:\n    command: php artisan export:work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &config.Site{Name: "myapp", Path: t.TempDir(), Framework: "testfw"}, wt
}
