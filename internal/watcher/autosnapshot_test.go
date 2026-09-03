package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// notified records the finished-run reports a pass handed to the daemon.
var notified [][2]int

// seedAutoSnapshotSite registers a site whose .env points at a lerd engine, the
// shape DBTargetsFor resolves a database from.
func seedAutoSnapshotSite(t *testing.T, name, service, database, mode string) config.Site {
	t.Helper()
	dir := t.TempDir()
	env := "DB_CONNECTION=" + service + "\nDB_HOST=lerd-" + service + "\nDB_DATABASE=" + database + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	return config.Site{Name: name, Path: dir, PHPVersion: "8.4", AutoSnapshot: mode}
}

// autoSnapshotEnv points config and the stamp file at temp dirs and stubs the
// podman-facing seams, returning a recorder of the snapshots taken.
func autoSnapshotEnv(t *testing.T) *[]serviceops.SnapshotTarget {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	autoSnapshotStampPathFn = func() string { return filepath.Join(tmp, "auto-snapshot.stamps") }

	prevCreate, prevPrune := autoSnapshotCreate, autoSnapshotPrune
	prevSupported, prevRunning := autoSnapshotSupported, autoSnapshotRunning
	prevNotify := autoSnapshotNotify
	notified = nil
	autoSnapshotNotify = func(databases, sites int) { notified = append(notified, [2]int{databases, sites}) }

	var taken []serviceops.SnapshotTarget
	autoSnapshotCreate = func(target serviceops.SnapshotTarget, name string, meta serviceops.SnapshotMeta) (*serviceops.Snapshot, error) {
		taken = append(taken, target)
		return &serviceops.Snapshot{Name: name + "-stamp", Service: target.Service, Database: target.Database, Auto: meta.Auto}, nil
	}
	autoSnapshotPrune = func(string, string, serviceops.RetentionPolicy) ([]string, error) { return nil, nil }
	autoSnapshotSupported = func(string) bool { return true }
	autoSnapshotRunning = func(string) bool { return true }

	t.Cleanup(func() {
		autoSnapshotStampPathFn = defaultAutoSnapshotStampPath
		autoSnapshotCreate, autoSnapshotPrune = prevCreate, prevPrune
		autoSnapshotSupported, autoSnapshotRunning = prevSupported, prevRunning
		autoSnapshotNotify = prevNotify
	})
	return &taken
}

func writeAutoSnapshotConfig(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(config.ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.GlobalConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunAutoSnapshots_takesAndThrottles(t *testing.T) {
	taken := autoSnapshotEnv(t)
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n  every: 24h\n")
	site := seedAutoSnapshotSite(t, "shop", "mysql", "shop_db", config.AutoSnapshotOn)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	now := time.Unix(2_000_000_000, 0)
	runAutoSnapshots(now)
	if len(*taken) != 1 {
		t.Fatalf("took %d snapshots, want 1 (%+v)", len(*taken), *taken)
	}
	if got := (*taken)[0]; got.Service != "mysql" || got.Database != "shop_db" {
		t.Errorf("target = %+v, want mysql/shop_db", got)
	}

	// Inside the schedule the stamp holds it back.
	runAutoSnapshots(now.Add(time.Hour))
	if len(*taken) != 1 {
		t.Errorf("took %d snapshots, want the schedule to throttle the second run", len(*taken))
	}
	// A full interval later it is due again.
	runAutoSnapshots(now.Add(25 * time.Hour))
	if len(*taken) != 2 {
		t.Errorf("took %d snapshots, want 2 once the interval elapsed", len(*taken))
	}
}

// The shipped default is on, but opt-in, so a machine nobody has configured
// gets no dumps it never asked for.
func TestRunAutoSnapshots_shippedDefaultTakesNothing(t *testing.T) {
	taken := autoSnapshotEnv(t)
	site := seedAutoSnapshotSite(t, "shop", "mysql", "shop_db", config.AutoSnapshotDefault)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoSnapshotEnabled() {
		t.Fatal("the schedule should ship on")
	}
	runAutoSnapshots(time.Unix(2_000_000_000, 0))
	if len(*taken) != 0 {
		t.Errorf("took %+v, want nothing until a site opts in", *taken)
	}
}

// A finished pass reports once, counting the databases it took and the sites
// they belong to; a pass that took nothing stays quiet.
func TestRunAutoSnapshots_reportsTheFinishedRun(t *testing.T) {
	autoSnapshotEnv(t)
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n  every: 24h\n")
	sites := []config.Site{
		seedAutoSnapshotSite(t, "shop", "mysql", "shop_db", config.AutoSnapshotOn),
		seedAutoSnapshotSite(t, "blog", "mysql", "blog_db", config.AutoSnapshotOn),
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: sites}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	now := time.Unix(2_000_000_000, 0)
	runAutoSnapshots(now)
	if len(notified) != 1 || notified[0] != [2]int{2, 2} {
		t.Fatalf("reports = %v, want one report of 2 databases on 2 sites", notified)
	}

	// Throttled out, so there is nothing to report.
	runAutoSnapshots(now.Add(time.Hour))
	if len(notified) != 1 {
		t.Errorf("reports = %v, want no second report", notified)
	}
}

func TestRunAutoSnapshots_respectsSiteOverrides(t *testing.T) {
	taken := autoSnapshotEnv(t)
	// Opt-in: only the site that said yes is taken.
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n  selection: opt_in\n")
	sites := []config.Site{
		seedAutoSnapshotSite(t, "in", "mysql", "in_db", config.AutoSnapshotOn),
		seedAutoSnapshotSite(t, "follows", "mysql", "follows_db", config.AutoSnapshotDefault),
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: sites}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	runAutoSnapshots(time.Unix(2_000_000_000, 0))
	if len(*taken) != 1 || (*taken)[0].Database != "in_db" {
		t.Fatalf("took %+v, want only the opted-in site's database", *taken)
	}
}

// Turning the schedule off stops every site, including one that opted in: the
// switch would otherwise promise something it does not do.
func TestRunAutoSnapshots_disabledStopsEvenAnOptedInSite(t *testing.T) {
	taken := autoSnapshotEnv(t)
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: false\n")
	site := seedAutoSnapshotSite(t, "in", "mysql", "in_db", config.AutoSnapshotOn)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	runAutoSnapshots(time.Unix(2_000_000_000, 0))
	if len(*taken) != 0 {
		t.Errorf("took %+v, want nothing while the schedule is off", *taken)
	}
}

func TestRunAutoSnapshots_skipsOptedOutAndIgnoredSites(t *testing.T) {
	taken := autoSnapshotEnv(t)
	// Opt-out, so an ignored site is skipped for being ignored rather than for
	// having said nothing.
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n  selection: opt_out\n")
	out := seedAutoSnapshotSite(t, "out", "mysql", "out_db", config.AutoSnapshotOff)
	ignored := seedAutoSnapshotSite(t, "ignored", "mysql", "ignored_db", config.AutoSnapshotDefault)
	ignored.Ignored = true
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{out, ignored}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	runAutoSnapshots(time.Unix(2_000_000_000, 0))
	if len(*taken) != 0 {
		t.Errorf("took %+v, want nothing", *taken)
	}
}

// Two sites on one database are one target: the same data must not be dumped twice.
func TestRunAutoSnapshots_dedupesSharedDatabase(t *testing.T) {
	taken := autoSnapshotEnv(t)
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n")
	sites := []config.Site{
		seedAutoSnapshotSite(t, "main", "mysql", "shared", config.AutoSnapshotOn),
		seedAutoSnapshotSite(t, "admin", "mysql", "shared", config.AutoSnapshotOn),
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: sites}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	runAutoSnapshots(time.Unix(2_000_000_000, 0))
	if len(*taken) != 1 {
		t.Errorf("took %d snapshots, want 1 for a shared database", len(*taken))
	}
}

func TestRunAutoSnapshots_skipsStoppedEngine(t *testing.T) {
	taken := autoSnapshotEnv(t)
	autoSnapshotRunning = func(string) bool { return false }
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n")
	site := seedAutoSnapshotSite(t, "shop", "mysql", "shop_db", config.AutoSnapshotOn)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	runAutoSnapshots(time.Unix(2_000_000_000, 0))
	if len(*taken) != 0 {
		t.Errorf("took %+v, want nothing while the engine is stopped", *taken)
	}
	if _, err := os.Stat(autoSnapshotStampPathFn()); err == nil {
		t.Error("a skipped target should not be stamped")
	}
}

// A failed dump must not stamp, or a transient engine error would cost a whole
// schedule instead of the next tick.
func TestRunAutoSnapshots_failureIsRetried(t *testing.T) {
	taken := autoSnapshotEnv(t)
	calls := 0
	autoSnapshotCreate = func(target serviceops.SnapshotTarget, name string, meta serviceops.SnapshotMeta) (*serviceops.Snapshot, error) {
		calls++
		*taken = append(*taken, target)
		return nil, os.ErrDeadlineExceeded
	}
	writeAutoSnapshotConfig(t, "auto_snapshot:\n  enabled: true\n  every: 24h\n")
	site := seedAutoSnapshotSite(t, "shop", "mysql", "shop_db", config.AutoSnapshotOn)
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	now := time.Unix(2_000_000_000, 0)
	runAutoSnapshots(now)
	runAutoSnapshots(now.Add(time.Minute))
	if calls != 2 {
		t.Errorf("create called %d times, want the next tick to retry after a failure", calls)
	}
}

func TestAutoSnapshotStamps_roundTrip(t *testing.T) {
	dir := t.TempDir()
	autoSnapshotStampPathFn = func() string { return filepath.Join(dir, "auto-snapshot.stamps") }
	t.Cleanup(func() { autoSnapshotStampPathFn = defaultAutoSnapshotStampPath })

	if len(loadAutoSnapshotStamps()) != 0 {
		t.Fatal("expected no stamps before the first write")
	}
	at := time.Unix(1_700_000_000, 0)
	saveAutoSnapshotStamps(map[string]time.Time{"mysql\x00shop": at})
	got := loadAutoSnapshotStamps()
	if !got["mysql\x00shop"].Equal(at) {
		t.Errorf("stamp = %v, want %v", got["mysql\x00shop"], at)
	}
}
