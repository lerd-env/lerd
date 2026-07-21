package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/power"
)

// fwWithOctane builds a minimal Laravel-like framework with a FrankenPHP worker
// entrypoint and its watch variant, matching the shape of the built-in Laravel
// definition.
func fwWithOctane() *Framework {
	return &Framework{
		Name:      "laravel",
		PublicDir: "public",
		FrankenPHP: &FrameworkFrankenPHP{
			Entrypoint:       []string{"frankenphp", "php-server", "-l", ":8000", "-r", "public/"},
			WorkerEntrypoint: []string{"sh", "-c", "exec php artisan octane:start --server=frankenphp --workers=auto"},
			WorkerReloadEntrypoint: []string{"sh", "-c",
				"exec php artisan octane:start --server=frankenphp --workers=auto --watch"},
			SupportsWorker: true,
		},
	}
}

func TestResolveFrankenPHPWorkerEntrypoint(t *testing.T) {
	fw := fwWithOctane()

	t.Run("non-worker returns normal entrypoint", func(t *testing.T) {
		dir := t.TempDir()
		got := fw.ResolveFrankenPHPWorkerEntrypoint(dir, false)
		if strings.Join(got, " ") != strings.Join(fw.FrankenPHP.Entrypoint, " ") {
			t.Fatalf("expected normal entrypoint, got %v", got)
		}
	})

	t.Run("worker without reload opt-in keeps standard worker entrypoint", func(t *testing.T) {
		dir := t.TempDir()
		got := fw.ResolveFrankenPHPWorkerEntrypoint(dir, true)
		if strings.Join(got, " ") != strings.Join(fw.FrankenPHP.WorkerEntrypoint, " ") {
			t.Fatalf("expected standard worker entrypoint, got %v", got)
		}
	})

	t.Run("reload opt-in without chokidar falls back to standard", func(t *testing.T) {
		dir := t.TempDir()
		if err := SetProjectWorkerReload(dir, "octane", true); err != nil {
			t.Fatal(err)
		}
		got := fw.ResolveFrankenPHPWorkerEntrypoint(dir, true)
		if strings.Join(got, " ") != strings.Join(fw.FrankenPHP.WorkerEntrypoint, " ") {
			t.Fatalf("expected fallback to standard entrypoint when chokidar absent, got %v", got)
		}
	})

	t.Run("reload opt-in with chokidar selects watch variant", func(t *testing.T) {
		dir := t.TempDir()
		if err := SetProjectWorkerReload(dir, "octane", true); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "node_modules", "chokidar"), 0o755); err != nil {
			t.Fatal(err)
		}
		got := fw.ResolveFrankenPHPWorkerEntrypoint(dir, true)
		joined := strings.Join(got, " ")
		if !strings.Contains(joined, "octane:start") || !strings.Contains(joined, "--watch") {
			t.Fatalf("expected watch variant, got %v", got)
		}
		// --poll is appended only where the container can't see host fs events.
		if WatcherNeedsPolling(dir) {
			if !strings.HasSuffix(got[len(got)-1], "--poll") {
				t.Fatalf("expected --poll appended on polling host, got %v", got)
			}
		} else {
			if strings.Contains(joined, "--poll") {
				t.Fatalf("did not expect --poll on inotify host, got %v", got)
			}
		}
	})

	t.Run("no watch variant defined keeps standard even when opted in", func(t *testing.T) {
		dir := t.TempDir()
		if err := SetProjectWorkerReload(dir, "octane", true); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "node_modules", "chokidar"), 0o755); err != nil {
			t.Fatal(err)
		}
		bare := &Framework{
			Name:      "laravel",
			PublicDir: "public",
			FrankenPHP: &FrameworkFrankenPHP{
				WorkerEntrypoint: []string{"sh", "-c", "exec php artisan octane:start"},
				SupportsWorker:   true,
			},
		}
		got := bare.ResolveFrankenPHPWorkerEntrypoint(dir, true)
		if strings.Join(got, " ") != strings.Join(bare.FrankenPHP.WorkerEntrypoint, " ") {
			t.Fatalf("expected standard entrypoint when no watch variant, got %v", got)
		}
	})
}

// TestSiteFrankenPHPQuadletSpec guards the shared resolver both quadlet writers
// (siteops.FinishFrankenPHPLink and the global install refresh) now go through,
// so the install path can't drop --watch for a site that opted into reload.
// Sandboxing the store dir forces GetFrameworkForDir onto the built-in Laravel
// definition, which carries WorkerReloadEntrypoint.
func TestSiteFrankenPHPQuadletSpec(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	dir := t.TempDir()
	site := &Site{Name: "demo", Path: dir, Framework: "laravel", Runtime: "frankenphp", RuntimeWorker: true}

	ep, _ := site.FrankenPHPQuadletSpec()
	if strings.Contains(strings.Join(ep, " "), "--watch") {
		t.Fatalf("did not expect --watch without reload opt-in, got %v", ep)
	}

	if err := SetProjectWorkerReload(dir, "octane", true); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "chokidar"), 0o755); err != nil {
		t.Fatal(err)
	}
	ep, _ = site.FrankenPHPQuadletSpec()
	if !strings.Contains(strings.Join(ep, " "), "--watch") {
		t.Fatalf("expected --watch after opt-in with chokidar, got %v", ep)
	}
}

func TestAppendPollFlag(t *testing.T) {
	t.Run("sh -c form appends inside the script", func(t *testing.T) {
		got := appendPollFlag([]string{"sh", "-c", "exec php artisan octane:start --watch"})
		want := "exec php artisan octane:start --watch --poll"
		if len(got) != 3 || got[2] != want {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("bare argv form appends a trailing arg", func(t *testing.T) {
		got := appendPollFlag([]string{"php", "artisan", "octane:start", "--watch"})
		if len(got) != 5 || got[4] != "--poll" {
			t.Fatalf("got %v", got)
		}
	})
}

// chokidar's own default is 100ms, which is what made a polling watcher stat
// every watched file ten times a second across the VM boundary. The point of
// the override is to be materially slower than that.
func TestWatcherPollEnv_TracksPollingDecision(t *testing.T) {
	dir := t.TempDir()
	got := WatcherPollEnv(dir)

	if !WatcherNeedsPolling(dir) {
		if got != "" {
			t.Fatalf("no polling on this host, want no env override, got %q", got)
		}
		return
	}

	const prefix = "CHOKIDAR_INTERVAL="
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("polling host must set the interval override, got %q", got)
	}
	ms, err := strconv.Atoi(strings.TrimPrefix(got, prefix))
	if err != nil {
		t.Fatalf("interval must be a plain integer of milliseconds, got %q", got)
	}
	if ms <= 100 {
		t.Errorf("interval %dms is not looser than chokidar's 100ms default", ms)
	}
}

// The ladder: a laptop on battery backs off, and Low Power Mode, an explicit
// request for less background work, backs off twice as far again.
func TestWatcherPollIntervalFor(t *testing.T) {
	mains := watcherPollIntervalFor(power.Mains)
	battery := watcherPollIntervalFor(power.Battery)
	low := watcherPollIntervalFor(power.LowPower)

	if battery != mains*2 {
		t.Errorf("battery interval = %d, want double mains (%d)", battery, mains*2)
	}
	if low != mains*4 {
		t.Errorf("low power interval = %d, want quadruple mains (%d)", low, mains*4)
	}
	if mains <= 100 {
		t.Errorf("mains interval %dms is not looser than chokidar's 100ms default", mains)
	}
}
