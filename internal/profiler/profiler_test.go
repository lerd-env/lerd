package profiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func tempXDG(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
}

func TestSetProfiling_GlobalToggle(t *testing.T) {
	tempXDG(t)
	old, oldServed := nginxReloadFn, servedStateFn
	nginxReloadFn = func() error { return nil }
	// No nginx here, so answer the readiness probe from the config the toggle
	// just wrote rather than waiting out the deadline on every call.
	servedStateFn = func() (bool, bool) {
		cfg, err := config.LoadGlobal()
		return err == nil && cfg.IsProfilerEnabled(), true
	}
	defer func() { nginxReloadFn, servedStateFn = old, oldServed }()

	if err := config.AddSite(config.Site{
		Name: "myapp", Domains: []string{"myapp.test"},
		Path: "/srv/myapp", PHPVersion: "8.3",
	}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}

	res, err := SetProfiling(true)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !res.Enabled || res.NoChange {
		t.Errorf("expected enabled, got %+v", res)
	}
	if cfg, _ := config.LoadGlobal(); cfg == nil || !cfg.IsProfilerEnabled() {
		t.Errorf("profiler flag not persisted")
	}
	conf, err := os.ReadFile(filepath.Join(config.NginxConfD(), "myapp.test.conf"))
	if err != nil {
		t.Fatalf("read vhost: %v", err)
	}
	if !strings.Contains(string(conf), "SPX_ENABLED=1") {
		t.Errorf("vhost missing SPX_ENABLED after enable:\n%s", conf)
	}

	if res2, _ := SetProfiling(true); !res2.NoChange {
		t.Errorf("second enable should report no-change")
	}

	if _, err := SetProfiling(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	conf2, _ := os.ReadFile(filepath.Join(config.NginxConfD(), "myapp.test.conf"))
	if strings.Contains(string(conf2), "SPX_ENABLED=1") {
		t.Errorf("vhost still injects SPX_ENABLED after disable:\n%s", conf2)
	}
}

// Arming has to hold the caller until nginx is serving the new configuration.
// The reload only signals nginx: it drains the old workers while the new ones
// come up, so a request sent the moment SetProfiling returns could still be
// served by the configuration that has no profiler attached.
func TestSetProfiling_WaitsUntilNginxServesTheNewState(t *testing.T) {
	tempXDG(t)
	oldReload, oldServed, oldPoll := nginxReloadFn, servedStateFn, servingPoll
	reloadedAt := 0
	calls := 0
	nginxReloadFn = func() error { reloadedAt = calls; return nil }
	// The first two probes still see the old configuration, as they would while
	// the old workers drain.
	servedStateFn = func() (bool, bool) {
		calls++
		if calls < 3 {
			return false, true
		}
		return true, true
	}
	servingPoll = time.Millisecond
	defer func() { nginxReloadFn, servedStateFn, servingPoll = oldReload, oldServed, oldPoll }()

	if _, err := SetProfiling(true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if calls < 3 {
		t.Errorf("returned after %d probes, should have waited for the state to flip", calls)
	}
	if reloadedAt != 0 {
		t.Errorf("probed before the reload was issued")
	}

	// The marker travels with the vhost the reload picks up, so it has to be
	// rewritten before nginx is signalled, not after.
	conf, err := os.ReadFile(filepath.Join(config.NginxConfD(), "_profiler.conf"))
	if err != nil {
		t.Fatalf("read profiler vhost: %v", err)
	}
	if !strings.Contains(string(conf), `return 200 "on"`) {
		t.Errorf("profiler vhost marker not updated on arm:\n%s", conf)
	}
}

// A probe that never confirms (nginx down, or a vhost that predates the marker)
// must not fail the toggle or hang the caller: the setting was still applied.
func TestSetProfiling_GivesUpWaitingWithoutFailing(t *testing.T) {
	tempXDG(t)
	oldReload, oldServed, oldTimeout, oldPoll := nginxReloadFn, servedStateFn, servingTimeout, servingPoll
	nginxReloadFn = func() error { return nil }
	servedStateFn = func() (bool, bool) { return false, false }
	servingTimeout, servingPoll = 10*time.Millisecond, time.Millisecond
	defer func() {
		nginxReloadFn, servedStateFn, servingTimeout, servingPoll = oldReload, oldServed, oldTimeout, oldPoll
	}()

	done := make(chan error, 1)
	go func() { _, err := SetProfiling(true); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("enable: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetProfiling hung waiting for a state that never arrives")
	}
}

func TestClearData(t *testing.T) {
	tempXDG(t)
	dir := config.SpxDataDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"report-1.json", "report-2.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	n, err := ClearData()
	if err != nil {
		t.Fatalf("ClearData: %v", err)
	}
	if n != 3 {
		t.Errorf("removed = %d, want 3", n)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("data dir not empty after clear: %d entries", len(entries))
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("data dir should survive the clear: %v", err)
	}
}

func TestClearData_MissingDirIsNotAnError(t *testing.T) {
	tempXDG(t)
	n, err := ClearData()
	if err != nil {
		t.Fatalf("ClearData on missing dir: %v", err)
	}
	if n != 0 {
		t.Errorf("removed = %d, want 0", n)
	}
}

func TestProfilable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		site config.Site
		want bool
	}{
		{"php fpm site", config.Site{Path: dir, PHPVersion: "8.4"}, true},
		{"custom per-site fpm image", config.Site{Path: dir, PHPVersion: "8.4", Runtime: "fpm-custom"}, true},
		{"frankenphp site", config.Site{Path: dir, PHPVersion: "8.4", Runtime: "frankenphp"}, false},
		{"host-proxy site", config.Site{Path: dir, HostPort: 5173}, false},
		{"custom container site", config.Site{Path: dir, ContainerPort: 3000}, false},
		{"static site", config.Site{Path: t.TempDir()}, false},
	}
	for _, tc := range cases {
		if got := Profilable(tc.site); got != tc.want {
			t.Errorf("Profilable(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
