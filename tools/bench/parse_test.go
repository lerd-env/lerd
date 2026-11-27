package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParsePSTime(t *testing.T) {
	cases := map[string]time.Duration{
		"00:00:09":   9 * time.Second,
		"01:02:03":   time.Hour + 2*time.Minute + 3*time.Second,
		"2-00:00:01": 48*time.Hour + time.Second,
		"0:01.23":    1230 * time.Millisecond, // macOS
		"12:00.50":   12*time.Minute + 500*time.Millisecond,
	}
	for in, want := range cases {
		got, err := parsePSTime(in)
		if err != nil || got != want {
			t.Errorf("parsePSTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "12", "a:b", "x-00:01"} {
		if _, err := parsePSTime(bad); err == nil {
			t.Errorf("parsePSTime(%q) accepted bad input", bad)
		}
	}
}

func TestParseProcStat(t *testing.T) {
	// comm may contain spaces and parens, so fields count from the last ')'.
	// utime 250 + stime 50 + reaped children 30 + 20 ticks: lerd's podman and
	// systemctl subprocesses are part of what it costs.
	stat := "4208 (lerd (ui) x) S 1 4208 4208 0 -1 4194560 100 0 0 0 250 50 30 20 20 0 30 0"
	got, err := parseProcStat(stat)
	if err != nil || got != 3500*time.Millisecond {
		t.Fatalf("parseProcStat = %v, %v; want 3.5s", got, err)
	}
	if _, err := parseProcStat("garbage"); err == nil {
		t.Fatal("parseProcStat accepted garbage")
	}
}

func TestParseSizeMB(t *testing.T) {
	cases := map[string]float64{"193.8MB": 193.8, "1.5GB": 1500, "512kB": 0.512, "1GiB": 1073.741824, "0B": 0}
	for in, want := range cases {
		got, err := parseSizeMB(in)
		if err != nil || got < want-1e-9 || got > want+1e-9 {
			t.Errorf("parseSizeMB(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := parseSizeMB("lots"); err == nil {
		t.Error("parseSizeMB accepted bad input")
	}
}

func TestRoleOf(t *testing.T) {
	cases := map[string]string{
		"/home/u/.local/bin/lerd serve-ui":             "ui",
		"/home/u/.local/bin/lerd watch":                "watcher",
		"/home/u/.local/bin/lerd dns-serve --listen x": "dns",
		"/home/u/.local/bin/lerd-tray":                 "tray",
		"lerd mcp":                                     "",
		"lerd watchdog":                                "",
		"/usr/bin/vim lerd watch":                      "",
	}
	for in, want := range cases {
		if got := roleOf(in); got != want {
			t.Errorf("roleOf(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestDelta(t *testing.T) {
	if got := delta(100, 110); got != "+10.0%" {
		t.Errorf("delta = %s", got)
	}
	if got := delta(0, 5); got != "new" {
		t.Errorf("delta from zero = %s", got)
	}
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		key           string
		old, new, nse float64
		want          string
	}{
		{"idle.rss_mb.host.ui", 76, 120, 0.05, "REGRESSED"},
		{"idle.rss_mb.host.ui", 76, 82, 0.05, ""},  // under 10%
		{"idle.rss_mb.host.ui", 76, 100, 0.20, ""}, // within twice the noise
		{"idle.rss_mb.host.ui", 2.5, 4.0, 0, ""},   // under the 5 MB floor
		{"idle.cpu_pct.host.ui", 0.0, 0.1, 0, ""},  // jitter from zero
		{"idle.cpu_pct.host.watcher", 1.2, 3.0, 0.1, "REGRESSED"},
		{"http.rps_c16", 30000, 20000, 0.1, "REGRESSED"}, // fewer requests is worse
		{"http.rps_c16", 20000, 30000, 0.1, "improved"},
		{"idle.mem_mb.ctr.lerd-nginx", 120, 60, 0.05, "improved"},
		{"calib.single_mbps", 2500, 1000, 0, ""}, // the machine, not lerd
	}
	for _, c := range cases {
		if got := verdict(c.key, c.old, c.new, c.nse); got != c.want {
			t.Errorf("verdict(%s, %v→%v, noise %v) = %q; want %q", c.key, c.old, c.new, c.nse, got, c.want)
		}
	}
}

func TestSummarise(t *testing.T) {
	med, noise := summarise(map[string][]float64{"a": {10, 12, 8}, "z": {0, 0}})
	if med["a"] != 10 || noise["a"] != 0.4 {
		t.Errorf("a: median %v noise %v; want 10, 0.4", med["a"], noise["a"])
	}
	if med["z"] != 0 || noise["z"] != 0 {
		t.Errorf("z: median %v noise %v; want 0, 0", med["z"], noise["z"])
	}
}

func TestLowestPassing(t *testing.T) {
	levels := []float64{4, 2, 1, 0.5}
	if got := lowestPassing(levels, []float64{50, 80, 300, 90}, 250); got != 2 {
		t.Errorf("a pass below a miss counted: got %v, want 2", got)
	}
	if got := lowestPassing(levels, []float64{50, 60, 70, 80}, 250); got != 0.5 {
		t.Errorf("all pass: got %v, want 0.5", got)
	}
	if got := lowestPassing(levels, []float64{400, 50}, 250); got != -1 {
		t.Errorf("first misses: got %v, want -1", got)
	}
}

func TestCPUMaxLine(t *testing.T) {
	for cpus, want := range map[float64]string{0.25: "25000 100000", 2: "200000 100000"} {
		if got := cpuMaxLine(cpus); got != want {
			t.Errorf("cpuMaxLine(%v) = %q; want %q", cpus, got, want)
		}
	}
}

func TestSetEnv(t *testing.T) {
	path := t.TempDir() + "/.env"
	os.WriteFile(path, []byte("APP_NAME=x\nSESSION_DRIVER=database\n# SESSION_DRIVER=old\n"), 0o644)
	if err := setEnv(path, map[string]string{"SESSION_DRIVER": "array", "CACHE_STORE": "array"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "APP_NAME=x\nSESSION_DRIVER=array\n# SESSION_DRIVER=old\nCACHE_STORE=array\n"
	if string(got) != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if err := setEnv(t.TempDir()+"/missing", map[string]string{"A": "b"}); err == nil {
		t.Error("missing .env accepted")
	}
}

func TestVerdict_zeroBaselineAndFailedTargets(t *testing.T) {
	cases := []struct {
		key           string
		old, new, nse float64
		want          string
	}{
		{"idle.cpu_pct.ctr.lerd-redis", 0, 20, 0, "REGRESSED"}, // a new steady drain from nothing
		{"idle.cpu_pct.ctr.lerd-redis", 0, 0.2, 0, ""},         // jitter under the floor
		{"idle.cpu_pct.ctr.lerd-redis", 20, 0, 0, "improved"},
		{"req.min_cpus", 1, -1, 0, "REGRESSED"}, // even four cores now fail
		{"req.min_cpus", -1, 1, 0, "improved"},  // it passes again
		{"req.min_cpus", -1, -1, 0, ""},
		{"req.min_cpu_score", 1200, -2400, 0, "REGRESSED"},
	}
	for _, c := range cases {
		if got := verdict(c.key, c.old, c.new, c.nse); got != c.want {
			t.Errorf("verdict(%s, %v→%v) = %q; want %q", c.key, c.old, c.new, got, c.want)
		}
	}
}

func TestRoleOf_nativePHP(t *testing.T) {
	if got := roleOf("/Users/u/.local/share/lerd/bin/php-native-fpm-8.4 --nodaemonize -y x.conf"); got != "native-fpm" {
		t.Errorf("roleOf(native fpm) = %q; want native-fpm", got)
	}
}

func TestSharersOf(t *testing.T) {
	sites := "Name  Domain  PHP  Node  TLS  Framework  Status  Path\n" +
		"bench-laravel  bench-laravel.test  8.4  22  No  Laravel 13  running  /h/Lerd/bench-laravel\n" +
		"bench-fixture  bench-fixture.test  8.4  22  No    running  /h/Lerd/bench-fixture\n" +
		"shop  shop.test  8.4  22  Yes  Laravel 12  running  /h/shop\n" +
		"blog  blog.test  8.5  22  No  Laravel 12  running  /h/blog\n" +
		"↳ alias  crisu.test\n"
	old := fixtureRoot
	fixtureRoot = "/h/Lerd"
	t.Cleanup(func() { fixtureRoot = old })
	// A user's own site under a fixture's name, elsewhere, is still the user's.
	sites += "bench-fixture  bench-fixture.local  8.4  22  No    running  /home/u/work/bench-fixture\n"
	// The run's own fixtures are not other sites, whichever PHP they landed on.
	if got := sharersOf(sites, "8.4"); len(got) != 2 || got[0] != "shop" || got[1] != "bench-fixture" {
		t.Errorf("sharersOf = %v; want [shop bench-fixture]", got)
	}
	if got := userSiteCount(sites); got != 3 {
		t.Errorf("userSiteCount = %d; want 3, the bench's fixtures and the alias left out", got)
	}
	if got := sharersOf(sites, "8.3"); len(got) != 0 {
		t.Errorf("sharersOf on an unused version = %v; want none", got)
	}
}

// Ctrl+C during the restore must wait for it rather than exit with a
// container still throttled, and nothing may limit a container after it.
func TestCleanups_runOnceAndMakeLateCallersWait(t *testing.T) {
	var c cleanups
	var order []string
	release := make(chan struct{})
	c.add(func() { order = append(order, "first added") })
	c.add(func() { <-release; order = append(order, "last added") })
	done := make(chan struct{})
	go func() { c.run(); close(done) }()
	time.Sleep(20 * time.Millisecond)
	second := make(chan struct{})
	go func() { c.run(); close(second) }()
	select {
	case <-second:
		t.Fatal("a second run returned before the first finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	<-done
	<-second
	if len(order) != 2 || order[0] != "last added" {
		t.Errorf("ran %v; want each once, newest first", order)
	}
}

func TestFixtureDir_refusesAFolderTheBenchDidNotMake(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php mine();"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := claimFixtureDir(dir); err == nil {
		t.Fatal("claimed a folder holding someone else's project")
	}
	fresh := filepath.Join(t.TempDir(), "bench-fixture")
	if err := claimFixtureDir(fresh); err != nil {
		t.Fatalf("a missing folder must be claimable: %v", err)
	}
	if err := claimFixtureDir(fresh); err != nil {
		t.Fatalf("the bench's own folder must stay claimable: %v", err)
	}
}

// A Ctrl+C that lands while a site is being linked must still unlink it, so
// the action and the registration of its undo happen as one step.
func TestCleanups_guardRegistersTheUndoOfAnActionInFlight(t *testing.T) {
	var c cleanups
	started, finish := make(chan struct{}), make(chan struct{})
	undone := false
	go func() {
		_ = c.guard(func() (func(), error) {
			close(started)
			<-finish
			return func() { undone = true }, nil
		})
	}()
	<-started
	ran := make(chan struct{})
	go func() { c.run(); close(ran) }()
	time.Sleep(20 * time.Millisecond)
	close(finish)
	<-ran
	if !undone {
		t.Fatal("a run during the action skipped its undo")
	}
	if err := c.guard(func() (func(), error) { t.Error("an action ran after cleanup"); return nil, nil }); err == nil {
		t.Error("guard after cleanup did not refuse")
	}
}

// On Silverblue /home is a symlink to /var/home, so lerd may list a fixture
// under either spelling of the same folder.
func TestBenchOwnedRow_followsSymlinkedHomes(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "Lerd", "bench-fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	old := fixtureRoot
	fixtureRoot = filepath.Join(link, "Lerd")
	t.Cleanup(func() { fixtureRoot = old })
	row := "bench-fixture  bench-fixture.test  8.4  22  No    running  " + filepath.Join(real, "Lerd", "bench-fixture")
	if !benchOwnedRow(row) {
		t.Error("the bench's fixture listed under its resolved path was not recognised")
	}
	if benchOwnedRow("bench-fixture  bench-fixture.test  8.4  22  No    running  /elsewhere/bench-fixture") {
		t.Error("a same-named site elsewhere was taken for the fixture")
	}
}
