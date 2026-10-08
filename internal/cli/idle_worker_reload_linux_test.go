package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
	"github.com/geodro/lerd/internal/siteinfo"
)

// unitTimeline records the order of reloads and unit operations a worker batch
// makes, so a test can see how many reloads it costs and where they fall.
type unitTimeline struct {
	services.ServiceManager
	mu     sync.Mutex
	events []string
	active map[string]bool
}

func (u *unitTimeline) log(e string) {
	u.mu.Lock()
	u.events = append(u.events, e)
	u.mu.Unlock()
}

func (u *unitTimeline) Start(name string) error   { u.log("start " + name); return nil }
func (u *unitTimeline) Stop(name string) error    { u.log("stop " + name); return nil }
func (u *unitTimeline) Restart(name string) error { u.log("restart " + name); return nil }
func (u *unitTimeline) Enable(string) error       { return nil }
func (u *unitTimeline) Disable(string) error      { return nil }
func (u *unitTimeline) UnitStatus(name string) (string, error) {
	if u.active[name] {
		return "active", nil
	}
	return "inactive", nil
}
func (u *unitTimeline) AllUnitStates() map[string]string { return nil }

func (u *unitTimeline) count(e string) int {
	n := 0
	for _, got := range u.events {
		if got == e {
			n++
		}
	}
	return n
}

func installUnitTimeline(t *testing.T) (*unitTimeline, *config.Site) {
	t.Helper()
	tmp := t.TempDir()
	// macOS writes worker units under ~/Library/LaunchAgents.
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "site")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: laravel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("STRIPE_SECRET=sk_test_x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	proj, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	proj.CustomWorkers = map[string]config.FrameworkWorker{
		"queue":   {Command: "php artisan queue:work"},
		"horizon": {Command: "php artisan horizon"},
	}
	if err := config.SaveProjectConfig(dir, proj); err != nil {
		t.Fatal(err)
	}
	site := &config.Site{Name: "site", Framework: "laravel", Path: dir, PHPVersion: "8.4"}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{*site}}); err != nil {
		t.Fatal(err)
	}

	u := &unitTimeline{active: map[string]bool{"lerd-queue-site": true, "lerd-horizon-site": true, "lerd-stripe-site": true}}
	prevLC, prevMgr, prevReload, prevStates := podman.UnitLifecycle, services.Mgr, podman.DaemonReloadFn, unitStatesOKFn
	t.Cleanup(func() {
		podman.UnitLifecycle, services.Mgr, podman.DaemonReloadFn, unitStatesOKFn = prevLC, prevMgr, prevReload, prevStates
	})
	// Unit files still land in the temp XDG dirs through the real manager.
	u.ServiceManager = prevMgr
	podman.UnitLifecycle = u
	services.Mgr = u
	podman.DaemonReloadFn = func() error { u.log("reload"); return nil }
	unitStatesOKFn = func() (map[string]string, bool) { return map[string]string{}, true }
	t.Cleanup(func() { unitStatesOKFn = siteinfo.AllUnitStatesOK })
	return u, site
}

// A site's workers fall asleep together, and removing each unit used to cost a
// daemon-reload of its own.
func TestSuspendWorkersForIdle_reloadsOnceForTheBatch(t *testing.T) {
	u, site := installUnitTimeline(t)

	got := SuspendWorkersForIdle(site)
	if !reflect.DeepEqual(got, []string{"horizon", "queue", "stripe"}) {
		t.Fatalf("suspended %v, want horizon, queue and stripe", got)
	}
	if n := u.count("reload"); n != 1 {
		t.Fatalf("%d reloads, want 1 for the batch: %v", n, u.events)
	}
	if last := u.events[len(u.events)-1]; last != "reload" {
		t.Fatalf("reload must follow every stop: %v", u.events)
	}
}

// Waking writes every unit first, reloads once, then starts them, since a start
// before the reload would act on the stale cached unit.
func TestResumeWorkersForIdle_reloadsOnceBeforeStarting(t *testing.T) {
	u, site := installUnitTimeline(t)
	u.active = map[string]bool{}

	ResumeWorkersForIdle(site, []string{"horizon", "queue", "stripe"})
	if n := u.count("reload"); n != 1 {
		t.Fatalf("%d reloads, want 1 for the batch: %v", n, u.events)
	}
	var starts []string
	for _, e := range u.events {
		if strings.HasPrefix(e, "start ") {
			starts = append(starts, e)
		}
	}
	if !reflect.DeepEqual(starts, []string{"start lerd-horizon-site", "start lerd-queue-site", "start lerd-stripe-site"}) {
		t.Fatalf("starts = %v", starts)
	}
	if u.events[0] != "reload" {
		t.Fatalf("the reload must come before any start: %v", u.events)
	}
}

// Waking services owes systemd a reload for their boot arming; the workers that
// wake with them pay it in their own reload rather than one of its own.
func TestResumeWorkersForIdle_carriesADeferredReload(t *testing.T) {
	u, site := installUnitTimeline(t)
	u.active = map[string]bool{}
	podman.DeferDaemonReload()

	ResumeWorkersForIdle(site, []string{"queue"})
	if n := u.count("reload"); n != 1 {
		t.Fatalf("%d reloads, want the deferred one folded into the batch: %v", n, u.events)
	}
	if err := podman.DaemonReloadIfNeeded(false); err != nil {
		t.Fatal(err)
	}
	if n := u.count("reload"); n != 1 {
		t.Fatalf("the batch reload must settle what was owed: %v", u.events)
	}
}

// Waking a worker that conflicts with another tears the other down inside the
// same batch, so the conflict does not cost a reload of its own.
func TestResumeWorkersForIdle_conflictSharesTheBatchReload(t *testing.T) {
	u, site := installUnitTimeline(t)
	u.active = map[string]bool{}
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		t.Fatal(err)
	}
	proj.CustomWorkers["horizon"] = config.FrameworkWorker{Command: "php artisan horizon", ConflictsWith: []string{"queue"}}
	if err := config.SaveProjectConfig(site.Path, proj); err != nil {
		t.Fatal(err)
	}

	ResumeWorkersForIdle(site, []string{"horizon"})
	if n := u.count("reload"); n != 1 {
		t.Fatalf("%d reloads, want the conflict's teardown folded into the batch: %v", n, u.events)
	}
}

// A host-proxy dev server wakes in the same batch as the site's other workers
// rather than reloading for its own unit.
func TestResumeWorkersForIdle_hostProxySharesTheBatchReload(t *testing.T) {
	u, site := installUnitTimeline(t)
	u.active = map[string]bool{}
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		t.Fatal(err)
	}
	proj.Proxy = &config.ProxyConfig{Command: "npm run dev", Port: 3000}
	if err := config.SaveProjectConfig(site.Path, proj); err != nil {
		t.Fatal(err)
	}
	site.HostCommand = "npm run dev"

	ResumeWorkersForIdle(site, []string{"queue", hostProxyWorkerName})
	if n := u.count("reload"); n != 1 {
		t.Fatalf("%d reloads, want the dev server folded into the batch: %v", n, u.events)
	}
	if u.count("start "+hostProxyWorkerUnit(site.Name)) != 1 {
		t.Fatalf("the dev server must still start: %v", u.events)
	}
	if proj, _ := config.LoadProjectConfig(site.Path); slices.Contains(proj.Workers, hostProxyWorkerName) {
		t.Fatalf("the dev server must not be written into the project's workers: %v", proj.Workers)
	}
}
