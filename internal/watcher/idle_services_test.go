package watcher

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/idle"
)

// fakeServices stands in for the service containers: running is what is up,
// users and deps are who keeps each service awake, calls logs every operation.
type fakeServices struct {
	mu      sync.Mutex
	running map[string]bool
	users   map[string][]config.Site
	deps    map[string][]string
	pinned  map[string]bool
	calls   []string
}

func (f *fakeServices) log(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeServices) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func installFakeServices(t *testing.T) *fakeServices {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	f := &fakeServices{running: map[string]bool{}, users: map[string][]config.Site{}, deps: map[string][]string{}, pinned: map[string]bool{}}

	prevRunning, prevUsers, prevSuspend, prevWake := runningServices, serviceUsers, suspendService, wakeServices
	prevStale, prevPinned, prevIndex, prevDetect := serviceStale, servicePinned, wtIndex, detectWorktrees
	prevResume := resumeWorkers
	t.Cleanup(func() {
		runningServices, serviceUsers, suspendService, wakeServices = prevRunning, prevUsers, prevSuspend, prevWake
		serviceStale, servicePinned, wtIndex, detectWorktrees = prevStale, prevPinned, prevIndex, prevDetect
		resumeWorkers = prevResume
	})
	wtIndex = newWorktreeIndex()
	detectWorktrees = func(string, string) ([]gitpkg.Worktree, error) { return nil, nil }

	runningServices = func() []string {
		f.mu.Lock()
		defer f.mu.Unlock()
		var out []string
		for n, up := range f.running {
			if up {
				out = append(out, n)
			}
		}
		return out
	}
	serviceUsers = func(name string) ([]config.Site, []string) { return f.users[name], f.deps[name] }
	servicePinned = func(name string) bool { return f.pinned[name] }
	suspendService = func(name string) ([]string, error) {
		f.log("suspend " + name)
		stopped := append([]string{name}, f.deps[name]...)
		f.mu.Lock()
		for _, s := range stopped {
			f.running[s] = false
		}
		f.mu.Unlock()
		for _, s := range stopped {
			_ = config.SetServiceIdleSuspended(s, true)
		}
		return stopped, nil
	}
	wakeServices = func(names []string) error {
		for _, n := range names {
			f.log("wake " + n)
			f.mu.Lock()
			f.running[n] = true
			f.mu.Unlock()
			_ = config.SetServiceIdleSuspended(n, false)
		}
		return nil
	}
	serviceStale = func(name string) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return config.ServiceIsIdleSuspended(name) && f.running[name]
	}
	resumeWorkers = func(s *config.Site, _ []string) { f.log("resume workers " + s.Name) }
	return f
}

const svcTimeout = 30 * time.Minute

func idleTracker(now time.Time, idleFor map[string]time.Duration) *idle.Tracker {
	tr := idle.NewTracker(nil)
	for k, d := range idleFor {
		tr.TouchSite(k, now.Add(-d))
	}
	return tr
}

func TestTickServices_suspendsOnlyWhenEveryKeyIsIdle(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		idleFor map[string]time.Duration
		pinned  bool
		siteCfg config.Site
		want    bool
	}{
		{"all idle", map[string]time.Duration{"shop": time.Hour, "blog": time.Hour, "svc:mysql": time.Hour, "svc:phpmyadmin": time.Hour}, false, config.Site{Name: "blog"}, true},
		{"one site active", map[string]time.Duration{"shop": time.Hour, "blog": time.Minute, "svc:mysql": time.Hour, "svc:phpmyadmin": time.Hour}, false, config.Site{Name: "blog"}, false},
		{"its admin dashboard in use", map[string]time.Duration{"shop": time.Hour, "blog": time.Hour, "svc:mysql": time.Hour, "svc:phpmyadmin": time.Minute}, false, config.Site{Name: "blog"}, false},
		{"dashboard never seen starts a grace window", map[string]time.Duration{"shop": time.Hour, "blog": time.Hour, "svc:mysql": time.Hour}, false, config.Site{Name: "blog"}, false},
		{"pinned service", map[string]time.Duration{"shop": time.Hour, "blog": time.Hour, "svc:mysql": time.Hour, "svc:phpmyadmin": time.Hour}, true, config.Site{Name: "blog"}, false},
		{"a pinned site uses it", map[string]time.Duration{"shop": time.Hour, "blog": time.Hour, "svc:mysql": time.Hour, "svc:phpmyadmin": time.Hour}, false, config.Site{Name: "blog", Pinned: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := installFakeServices(t)
			f.running["mysql"] = true
			f.users["mysql"] = []config.Site{{Name: "shop"}, tc.siteCfg}
			f.deps["mysql"] = []string{"phpmyadmin"}
			f.pinned["mysql"] = tc.pinned

			e := newIdleEngine(idleTracker(now, tc.idleFor))
			e.tickServices(true, svcTimeout, now)
			e.wait()

			got := reflect.DeepEqual(f.callLog(), []string{"suspend mysql"})
			if got != tc.want {
				t.Fatalf("calls = %v, want suspended=%v", f.callLog(), tc.want)
			}
			if tc.want && !(e.sleeping["mysql"] && e.sleeping["phpmyadmin"]) {
				t.Fatalf("sleeping = %v, want mysql and its dependent", e.sleeping)
			}
		})
	}
}

func TestTickServices_worktreeActivityKeepsServiceAwake(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	f.running["redis"] = true
	f.users["redis"] = []config.Site{{Name: "shop", Path: "/srv/shop", Domains: []string{"shop.test"}}}
	detectWorktrees = func(string, string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Branch: "feat", Path: "/srv/shop-feat", Domain: "feat.shop.test"}}, nil
	}
	if err := config.AddSite(f.users["redis"][0]); err != nil {
		t.Fatal(err)
	}
	wtIndex.refresh()
	var wtBase string
	for _, wt := range wtIndex.forSite("shop") {
		wtBase = wt.Base
	}
	if wtBase == "" {
		t.Fatal("worktree index did not pick up the worktree")
	}

	e := newIdleEngine(idleTracker(now, map[string]time.Duration{
		"shop": time.Hour, "svc:redis": time.Hour, wtKey("shop", wtBase): time.Minute,
	}))
	e.tickServices(true, svcTimeout, now)
	e.wait()
	if len(f.callLog()) != 0 {
		t.Fatalf("a busy worktree let its service sleep: %v", f.callLog())
	}
}

func TestOnActivity_wakesTheSleepingServicesOfTheSite(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	f.users["mysql"] = []config.Site{{Name: "shop"}}
	f.users["meilisearch"] = []config.Site{{Name: "blog"}}
	_ = config.SetServiceIdleSuspended("mysql", true)
	_ = config.SetServiceIdleSuspended("meilisearch", true)

	e := newIdleEngine(idleTracker(now, map[string]time.Duration{"shop": time.Hour, "blog": time.Hour, "svc:mysql": time.Hour, "svc:meilisearch": time.Hour}))
	e.tickServices(true, svcTimeout, now) // learns who keeps each asleep service awake
	e.wait()
	if len(f.callLog()) != 0 {
		t.Fatalf("idle tick touched services: %v", f.callLog())
	}

	e.OnActivity("shop")
	e.wait()
	if !reflect.DeepEqual(f.callLog(), []string{"wake mysql"}) {
		t.Fatalf("calls = %v, want only shop's mysql woken", f.callLog())
	}
	if e.sleeping["mysql"] || !e.sleeping["meilisearch"] {
		t.Fatalf("sleeping = %v", e.sleeping)
	}
}

func TestOnActivity_dashboardKeyWakesItsService(t *testing.T) {
	f := installFakeServices(t)
	_ = config.SetServiceIdleSuspended("phpmyadmin", true)
	e := newIdleEngine(idle.NewTracker(nil))

	e.OnActivity(svcKey("phpmyadmin"))
	e.wait()
	if !reflect.DeepEqual(f.callLog(), []string{"wake phpmyadmin"}) {
		t.Fatalf("calls = %v", f.callLog())
	}
}

func TestResume_wakesServicesBeforeWorkers(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	if err := config.AddSite(config.Site{Name: "shop", Path: "/srv/shop", Domains: []string{"shop.test"}, IdleSuspendedWorkers: []string{"queue"}}); err != nil {
		t.Fatal(err)
	}
	f.users["redis"] = []config.Site{{Name: "shop"}}
	_ = config.SetServiceIdleSuspended("redis", true)

	e := newIdleEngine(idleTracker(now, map[string]time.Duration{"shop": time.Hour, "svc:redis": time.Hour}))
	e.suspended["shop"] = true
	e.tickServices(true, svcTimeout, now)
	e.wait()

	e.resume("shop") // the tick's backstop path, with no OnActivity wake ahead of it
	e.wait()
	want := []string{"wake redis", "resume workers shop"}
	if !reflect.DeepEqual(f.callLog(), want) {
		t.Fatalf("calls = %v, want %v", f.callLog(), want)
	}
}

func TestTickServices_wakesWhatWasStartedBehindItsBack(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	f.users["mysql"] = []config.Site{{Name: "shop"}}
	_ = config.SetServiceIdleSuspended("mysql", true)
	f.running["mysql"] = true // `lerd artisan` started it through the CLI

	e := newIdleEngine(idleTracker(now, map[string]time.Duration{"shop": time.Hour, "svc:mysql": time.Hour}))
	e.tickServices(true, svcTimeout, now)
	e.wait()
	if !reflect.DeepEqual(f.callLog(), []string{"wake mysql"}) {
		t.Fatalf("calls = %v, want a formal wake so shop gets its vhost back", f.callLog())
	}
}

func TestTickServices_offWakesEverythingAsleep(t *testing.T) {
	f := installFakeServices(t)
	_ = config.SetServiceIdleSuspended("mysql", true)
	_ = config.SetServiceIdleSuspended("redis", true)
	f.running["mailpit"] = true

	e := newIdleEngine(idle.NewTracker(nil))
	e.tickServices(false, svcTimeout, time.Now())
	e.wait()
	if !reflect.DeepEqual(f.callLog(), []string{"wake mysql", "wake redis"}) {
		t.Fatalf("calls = %v", f.callLog())
	}
	if got := config.IdleSuspendedServices(); len(got) != 0 {
		t.Fatalf("still held asleep: %v", got)
	}
}

func TestTickServices_leavesAServiceTheUserStoppedAlone(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	f.users["mysql"] = []config.Site{{Name: "shop"}}
	_ = config.SetServiceIdleSuspended("mysql", true)
	e := newIdleEngine(idleTracker(now, map[string]time.Duration{"shop": time.Minute, "svc:mysql": time.Minute}))

	_ = config.SetServiceIdleSuspended("mysql", false) // `lerd service stop mysql` while it slept
	e.tickServices(true, svcTimeout, now)
	e.wait()
	if len(f.callLog()) != 0 {
		t.Fatalf("woke a service the user stopped: %v", f.callLog())
	}
}

func TestTickServices_leavesAServiceMidSuspendAlone(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	f.running["mysql"] = true
	f.users["mysql"] = []config.Site{{Name: "shop"}}
	_ = config.SetServiceIdleSuspended("mysql", true) // flagged, stop still running
	e := newIdleEngine(idleTracker(now, map[string]time.Duration{"shop": time.Hour, "svc:mysql": time.Hour}))
	e.inFlight[svcKey("mysql")] = true

	e.tickServices(true, svcTimeout, now)
	e.wait()
	if len(f.callLog()) != 0 {
		t.Fatalf("touched a service mid-suspend: %v", f.callLog())
	}
}

// A service started from outside (adminer opened from the dashboard) must get
// a full timeout, not be put back to sleep on the tick after it is reconciled.
func TestTickServices_wakeStartsAFreshCountdown(t *testing.T) {
	now := time.Now()
	f := installFakeServices(t)
	f.users["mysql"] = []config.Site{{Name: "shop"}}
	_ = config.SetServiceIdleSuspended("mysql", true)
	f.running["mysql"] = true
	e := newIdleEngine(idleTracker(now, map[string]time.Duration{"shop": time.Hour, "svc:mysql": time.Hour}))

	e.tickServices(true, svcTimeout, now) // reconciles the outside start
	e.wait()
	e.tickServices(true, svcTimeout, now.Add(time.Minute))
	e.wait()
	if got := f.callLog(); !reflect.DeepEqual(got, []string{"wake mysql"}) {
		t.Fatalf("calls = %v, want the wake and no suspend right after", got)
	}
}

// A brief wake for a snapshot leaves the service asleep: stopped again, still
// flagged, and the tick does not treat the running unit as stale meanwhile.
func TestWithServiceBriefly_startsRunsAndStopsWithoutWaking(t *testing.T) {
	f := installFakeServices(t)
	_ = config.SetServiceIdleSuspended("mysql", true)
	prevStart, prevStop := serviceStartRaw, serviceStopRaw
	t.Cleanup(func() { serviceStartRaw, serviceStopRaw = prevStart, prevStop })
	serviceStartRaw = func(n string) error { f.log("start " + n); return nil }
	serviceStopRaw = func(n string) error { f.log("stop " + n); return nil }

	e := newIdleEngine(idle.NewTracker(nil))
	err := e.withServiceBriefly("mysql", func() error {
		if !e.inFlight[svcKey("mysql")] {
			t.Error("not marked busy while running")
		}
		f.log("dump")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.callLog(); !reflect.DeepEqual(got, []string{"start mysql", "dump", "stop mysql"}) {
		t.Fatalf("calls = %v", got)
	}
	if !config.ServiceIsIdleSuspended("mysql") {
		t.Fatal("a snapshot wake cleared the sleeping flag")
	}
}
