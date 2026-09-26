package cli

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fakeIdleServices stands in for containers and nginx: up is the set of running
// services, users maps a service to the sites using it.
type fakeIdleServices struct {
	mu       sync.Mutex
	up       map[string]bool
	users    map[string][]config.Site
	deps     map[string][]string
	waking   []string
	restored []string
	stopped  []string
	reloads  int
}

func installFakeIdleServices(t *testing.T) *fakeIdleServices {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	f := &fakeIdleServices{up: map[string]bool{}, users: map[string][]config.Site{}, deps: map[string][]string{}}
	stop, ensure, up, using, swap, restore, reload, deps := idleStopService, idleEnsureService, idleServiceUp, idleSitesUsing, idleSwapToWaking, idleRestoreVhost, idleReloadNginx, idleDependentsOf
	t.Cleanup(func() {
		idleStopService, idleEnsureService, idleServiceUp, idleSitesUsing = stop, ensure, up, using
		idleSwapToWaking, idleRestoreVhost, idleReloadNginx, idleDependentsOf = swap, restore, reload, deps
	})
	idleStopService = func(name string) error {
		f.stopped = append(f.stopped, name)
		delete(f.up, name)
		for _, d := range f.deps[name] {
			delete(f.up, d)
		}
		return nil
	}
	idleEnsureService = func(name string) error {
		f.mu.Lock()
		f.up[name] = true
		f.mu.Unlock()
		return nil
	}
	idleServiceUp = func(name string) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.up[name]
	}
	idleSitesUsing = func(name string) []config.Site { return f.users[name] }
	idleSwapToWaking = func(s *config.Site) error { f.waking = append(f.waking, s.Name); return nil }
	idleRestoreVhost = func(s *config.Site) error { f.restored = append(f.restored, s.Name); return nil }
	idleReloadNginx = func() { f.reloads++ }
	idleDependentsOf = func(name string) []string { return f.deps[name] }
	return f
}

func TestSuspendServiceForIdle_WakingPageBeforeStopAndRecordsDependents(t *testing.T) {
	f := installFakeIdleServices(t)
	f.up["mysql"], f.up["phpmyadmin"] = true, true
	f.deps["mysql"] = []string{"phpmyadmin", "adminer"} // adminer installed but not running
	f.users["mysql"] = []config.Site{{Name: "shop"}, {Name: "blog"}}

	var wakingAtStop []string
	idleStopService = func(name string) error {
		wakingAtStop = append([]string(nil), f.waking...)
		f.up = map[string]bool{}
		return nil
	}

	stopped, err := SuspendServiceForIdle("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stopped, []string{"mysql", "phpmyadmin"}) {
		t.Fatalf("stopped = %v, want mysql and its running dependent only", stopped)
	}
	if !reflect.DeepEqual(wakingAtStop, []string{"shop", "blog"}) {
		t.Fatalf("sites on the waking page when mysql stopped = %v", wakingAtStop)
	}
	if got := config.IdleSuspendedServices(); !reflect.DeepEqual(got, []string{"mysql", "phpmyadmin"}) {
		t.Fatalf("held asleep = %v", got)
	}
}

func TestSuspendServiceForIdle_StopFailureHoldsNothingAsleep(t *testing.T) {
	f := installFakeIdleServices(t)
	f.up["redis"] = true
	idleStopService = func(string) error { return errors.New("boom") }

	if _, err := SuspendServiceForIdle("redis"); err == nil {
		t.Fatal("want the stop error back")
	}
	if got := config.IdleSuspendedServices(); len(got) != 0 {
		t.Fatalf("a service that never stopped is held asleep: %v", got)
	}
}

func TestWakeServicesForIdle_RestoresOnlySitesWithEverythingUp(t *testing.T) {
	f := installFakeIdleServices(t)
	shop, blog := config.Site{Name: "shop"}, config.Site{Name: "blog"}
	f.users["mysql"] = []config.Site{shop, blog}
	f.users["meilisearch"] = []config.Site{blog}
	for _, n := range []string{"mysql", "meilisearch"} {
		_ = config.SetServiceIdleSuspended(n, true)
	}

	if err := WakeServicesForIdle([]string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if !f.up["mysql"] || f.up["meilisearch"] {
		t.Fatalf("up = %v, want only mysql", f.up)
	}
	if !reflect.DeepEqual(f.restored, []string{"shop"}) {
		t.Fatalf("restored = %v, want shop only (blog still waits on meilisearch)", f.restored)
	}
	if got := config.IdleSuspendedServices(); !reflect.DeepEqual(got, []string{"meilisearch"}) {
		t.Fatalf("held asleep = %v", got)
	}
	if f.reloads != 1 {
		t.Fatalf("nginx reloads = %d, want 1", f.reloads)
	}
}

func TestWakeServicesForIdle_LeavesSleepingDevServerOnWakingPage(t *testing.T) {
	f := installFakeIdleServices(t)
	f.users["mysql"] = []config.Site{{Name: "spa", HostPort: 5173, IdleSuspendedWorkers: []string{hostProxyWorkerName}}}
	_ = config.SetServiceIdleSuspended("mysql", true)

	_ = WakeServicesForIdle([]string{"mysql"})
	if len(f.restored) != 0 {
		t.Fatalf("restored %v while its dev server still sleeps", f.restored)
	}
}

func TestWakeServicesForIdle_FailedStartStaysAsleep(t *testing.T) {
	f := installFakeIdleServices(t)
	f.users["redis"] = []config.Site{{Name: "shop"}}
	_ = config.SetServiceIdleSuspended("redis", true)
	idleEnsureService = func(string) error { return errors.New("image gone") }

	if err := WakeServicesForIdle([]string{"redis"}); err == nil {
		t.Fatal("want the start error back")
	}
	if !config.ServiceIsIdleSuspended("redis") || len(f.restored) != 0 {
		t.Fatalf("a service that failed to start was marked awake, restored=%v", f.restored)
	}
}

func TestIdleServiceStateIsStale(t *testing.T) {
	f := installFakeIdleServices(t)
	_ = config.SetServiceIdleSuspended("redis", true)
	if IdleServiceStateIsStale("redis") {
		t.Fatal("asleep and down is not stale")
	}
	f.up["redis"] = true
	if !IdleServiceStateIsStale("redis") {
		t.Fatal("asleep but running is stale")
	}
	f.up["mysql"] = true
	if IdleServiceStateIsStale("mysql") {
		t.Fatal("a service never put to sleep is not stale")
	}
}

func TestDropIdleSuspendedServiceUnits(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	_ = config.SetServiceIdleSuspended("mysql", true)

	got := dropIdleSuspendedServiceUnits([]string{"lerd-nginx", "lerd-mysql", "lerd-mysql-exporter", "lerd-redis"})
	want := []string{"lerd-nginx", "lerd-mysql-exporter", "lerd-redis"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIdleServicesCmd_savesTheSetting(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd := newIdleServicesCmd()
	for _, tc := range []struct {
		arg  string
		want bool
	}{{"on", true}, {"off", false}} {
		if err := cmd.RunE(cmd, []string{tc.arg}); err != nil {
			t.Fatal(err)
		}
		cfg, _ := config.LoadGlobal()
		if cfg.IdleSuspend.Services != tc.want {
			t.Fatalf("after %s: services = %v", tc.arg, cfg.IdleSuspend.Services)
		}
	}
	if err := cmd.RunE(cmd, []string{"maybe"}); err == nil {
		t.Fatal("an unknown value must be refused")
	}
}

func TestWakeServicesForIdle_startsServicesInParallel(t *testing.T) {
	installFakeIdleServices(t)
	for _, n := range []string{"mysql", "redis"} {
		_ = config.SetServiceIdleSuspended(n, true)
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	idleEnsureService = func(name string) error {
		started <- name
		<-release
		return nil
	}
	done := make(chan error)
	go func() { done <- WakeServicesForIdle([]string{"mysql", "redis"}) }()
	<-started
	<-started // both began before either finished
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := config.IdleSuspendedServices(); len(got) != 0 {
		t.Fatalf("still asleep: %v", got)
	}
}

func TestSuspendServiceForIdle_flagsBeforeStopping(t *testing.T) {
	f := installFakeIdleServices(t)
	f.up["redis"] = true
	var flaggedAtStop bool
	idleStopService = func(string) error {
		flaggedAtStop = config.ServiceIsIdleSuspended("redis")
		return nil
	}
	if _, err := SuspendServiceForIdle("redis"); err != nil {
		t.Fatal(err)
	}
	if !flaggedAtStop {
		t.Fatal("redis stopped before it was flagged asleep, so the dashboard would log it as stopped")
	}
}
