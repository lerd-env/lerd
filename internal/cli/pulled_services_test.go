package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func pulledFramework() *config.Framework {
	return &config.Framework{
		Name:  "pulledfw",
		Label: "Pulled",
		Env: config.FrameworkEnvConf{
			File: ".env",
			Services: map[string]config.FrameworkServiceDef{
				"mysql":       {Vars: []string{"DB_HOST=lerd-mysql"}},
				"postgres":    {Vars: []string{"DB_HOST=lerd-postgres"}},
				"redis":       {Vars: []string{"REDIS_HOST=lerd-redis"}},
				"meilisearch": {Vars: []string{"MEILISEARCH_HOST=http://lerd-meilisearch:7700"}},
				"rustfs":      {Vars: []string{"AWS_ENDPOINT=http://lerd-rustfs:9000"}},
			},
		},
	}
}

// pulledSite is a site whose .env points at mysql only, after a pull added
// services to its .lerd.yaml.
func pulledSite(t *testing.T, services ...config.ProjectService) *config.Site {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.SaveFramework(pulledFramework()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := config.SaveProjectConfig(dir, &config.ProjectConfig{Framework: "pulledfw", Services: services}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\nAWS_ENDPOINT=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &config.Site{Name: "demo", Path: dir, Framework: "pulledfw", WiredServices: []string{"mysql"}}
}

func TestUnwiredProjectServices(t *testing.T) {
	site := pulledSite(t,
		config.ProjectService{Name: "mysql"}, config.ProjectService{Name: "sqlite"},
		config.ProjectService{Name: "meilisearch"}, config.ProjectService{Name: "postgres"},
		config.ProjectService{Name: "rustfs", Preset: "rustfs"},
	)
	site.DeclinedServices = []string{"meilisearch"}
	// lerd wired postgres once and the env no longer names it: repointed by hand.
	site.WiredServices = append(site.WiredServices, "postgres")
	proj, _ := config.LoadProjectConfig(site.Path)

	got := serviceNamesOf(unwiredProjectServices(site, proj, pulledFramework()))
	if !slices.Equal(got, []string{"rustfs"}) {
		t.Fatalf("unwired = %v, want [rustfs]", got)
	}
}

// A pull can bring in a drop-in not installed on this machine yet, such as valkey
// standing in for redis. Only its preset says which role it fills.
func TestUnwiredProjectServices_SeesANotYetInstalledDropIn(t *testing.T) {
	site := pulledSite(t, config.ProjectService{Name: "mysql"}, config.ProjectService{Name: "valkey", Preset: "valkey"})
	proj, _ := config.LoadProjectConfig(site.Path)

	got := serviceNamesOf(unwiredProjectServices(site, proj, pulledFramework()))
	if !slices.Equal(got, []string{"valkey"}) {
		t.Fatalf("unwired = %v, want [valkey]", got)
	}
}

// stubPulledServiceSeams stubs starting services, provisioning site state and
// the `lerd env` re-exec, recording what was started and where env ran.
func stubPulledServiceSeams(t *testing.T) (started, envDirs *[]string) {
	t.Helper()
	stubLinkServiceSeams(t, nil)
	var s, e []string
	ensureServiceRunning = func(name string) error { s = append(s, name); return nil }
	prevEnv := runSiteEnv
	runSiteEnv = func(dir string) ([]byte, error) { e = append(e, dir); return nil, nil }
	t.Cleanup(func() { runSiteEnv = prevEnv })
	pulledServicesTried = map[string]pulledAttempt{}
	return &s, &e
}

// A pull that adds a service to .lerd.yaml has to start it and rewrite the env,
// or the dashboard lists it while the app still talks to nothing.
func TestWirePulledServices_StartsNewServiceAndRewritesEnv(t *testing.T) {
	site := pulledSite(t, config.ProjectService{Name: "mysql"}, config.ProjectService{Name: "rustfs", Preset: "rustfs"})
	started, envDirs := stubPulledServiceSeams(t)
	if err := config.SaveCustomService(&config.CustomService{Name: "rustfs", Image: "docker.io/rustfs/rustfs:latest"}); err != nil {
		t.Fatal(err)
	}

	applied := WirePulledServices(site)

	if !slices.Equal(applied, []string{"rustfs"}) {
		t.Errorf("applied = %v, want [rustfs]", applied)
	}
	if !slices.Equal(slices.Compact(*started), []string{"rustfs"}) {
		t.Errorf("started = %v, want only rustfs", *started)
	}
	if !slices.Equal(*envDirs, []string{site.Path}) {
		t.Errorf("env ran in %v, want once in the site folder", *envDirs)
	}

	// env rewrites .env and .lerd.yaml, which fires the watcher again; a service
	// env still did not wire must not set off another run.
	WirePulledServices(site)
	if len(*envDirs) != 1 {
		t.Errorf("env ran %d times for the same unwired set, want 1", len(*envDirs))
	}
}

// An inline container from a pulled repo runs whatever image and command the
// repo names, so it waits for someone to run `lerd link` and approve it.
func TestWirePulledServices_LeavesNewInlineServiceForLink(t *testing.T) {
	inline := config.ProjectService{Name: "rustfs", Custom: &config.CustomService{Image: "docker.io/someone/anything:latest"}}
	site := pulledSite(t, config.ProjectService{Name: "mysql"}, inline)
	started, envDirs := stubPulledServiceSeams(t)

	if applied := WirePulledServices(site); len(applied) != 0 {
		t.Errorf("applied = %v, want none", applied)
	}
	if len(*started) != 0 || len(*envDirs) != 0 {
		t.Errorf("started %v and ran env in %v, want nothing", *started, *envDirs)
	}
	if _, err := config.LoadCustomService("rustfs"); err == nil {
		t.Error("inline service was installed without approval")
	}
}

// A name the store does not know fails to fetch every time, and the watcher
// runs on each save of .env or composer.lock, so it is asked once per process.
func TestFetchMissingPresets_AsksTheStoreOncePerName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var asked []string
	prev := ensurePreset
	ensurePreset = func(name string) (*config.Preset, error) {
		asked = append(asked, name)
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { ensurePreset = prev })
	presetsFetched = map[string]bool{}

	proj := &config.ProjectConfig{Services: []config.ProjectService{{Name: "no-such-service"}}}
	fetchMissingPresets(proj)
	fetchMissingPresets(proj)

	if !slices.Equal(asked, []string{"no-such-service"}) {
		t.Errorf("asked the store for %v, want no-such-service once", asked)
	}
}

// A versioned entry names its preset separately, and only the preset says it
// stands in for mysql.
func TestUnwiredProjectServices_SeesANotYetInstalledVersionedDropIn(t *testing.T) {
	site := pulledSite(t, config.ProjectService{Name: "mariadb-11-8", Preset: "mariadb", PresetVersion: "11.8"})
	site.WiredServices = nil
	if err := os.WriteFile(filepath.Join(site.Path, ".env"), []byte("DB_HOST=127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj, _ := config.LoadProjectConfig(site.Path)

	got := serviceNamesOf(unwiredProjectServices(site, proj, pulledFramework()))
	if !slices.Equal(got, []string{"mariadb-11-8"}) {
		t.Fatalf("unwired = %v, want [mariadb-11-8]", got)
	}
}

// Pausing a site stops its services on purpose; a pull must not start them.
func TestWirePulledServices_LeavesAPausedSiteAlone(t *testing.T) {
	site := pulledSite(t, config.ProjectService{Name: "mysql"}, config.ProjectService{Name: "rustfs", Preset: "rustfs"})
	site.Paused = true
	started, envDirs := stubPulledServiceSeams(t)

	if applied := WirePulledServices(site); len(applied) != 0 || len(*started) != 0 || len(*envDirs) != 0 {
		t.Errorf("applied %v, started %v, env in %v, want nothing", applied, *started, *envDirs)
	}
}

// An install that failed (the store or registry was down) is tried again on a
// later change once the back-off has passed, not skipped for good.
func TestWirePulledServices_RetriesAnUnfinishedSetAfterABackOff(t *testing.T) {
	site := pulledSite(t, config.ProjectService{Name: "mysql"}, config.ProjectService{Name: "rustfs", Preset: "rustfs"})
	if err := config.SaveCustomService(&config.CustomService{Name: "rustfs", Image: "docker.io/rustfs/rustfs:latest"}); err != nil {
		t.Fatal(err)
	}
	_, envDirs := stubPulledServiceSeams(t)
	clock := time.Now()
	prevNow := pulledNow
	pulledNow = func() time.Time { return clock }
	t.Cleanup(func() { pulledNow = prevNow })

	WirePulledServices(site) // env is stubbed, so rustfs stays unwired
	WirePulledServices(site)
	if len(*envDirs) != 1 {
		t.Fatalf("env ran %d times inside the back-off, want 1", len(*envDirs))
	}
	clock = clock.Add(pulledRetryAfter + time.Second)
	WirePulledServices(site)
	if len(*envDirs) != 2 {
		t.Errorf("env ran %d times after the back-off, want 2", len(*envDirs))
	}
}

// env writes a service's keys whether or not its container came up, which would
// record it as wired and end the retries, so env waits until every one is running.
func TestWirePulledServices_HoldsEnvUntilTheServiceRuns(t *testing.T) {
	site := pulledSite(t, config.ProjectService{Name: "mysql"}, config.ProjectService{Name: "rustfs", Preset: "rustfs"})
	if err := config.SaveCustomService(&config.CustomService{Name: "rustfs", Image: "docker.io/rustfs/rustfs:latest"}); err != nil {
		t.Fatal(err)
	}
	_, envDirs := stubPulledServiceSeams(t)
	ensureServiceRunning = func(string) error { return os.ErrDeadlineExceeded }
	clock := time.Now()
	prevNow := pulledNow
	pulledNow = func() time.Time { return clock }
	t.Cleanup(func() { pulledNow = prevNow })

	if applied := WirePulledServices(site); len(applied) != 0 || len(*envDirs) != 0 {
		t.Fatalf("applied %v and ran env in %v while rustfs is down, want neither", applied, *envDirs)
	}
	ensureServiceRunning = func(string) error { return nil }
	clock = clock.Add(pulledRetryAfter + time.Second)
	if applied := WirePulledServices(site); !slices.Equal(applied, []string{"rustfs"}) || len(*envDirs) != 1 {
		t.Errorf("after recovery applied %v, env ran %d times, want [rustfs] once", applied, len(*envDirs))
	}
}
