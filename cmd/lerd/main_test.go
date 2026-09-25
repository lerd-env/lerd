package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/reqstats"
	"github.com/geodro/lerd/internal/siteops"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))
	for _, d := range []string{
		config.ConfigDir(),
		config.DataDir(),
		config.NginxConfD(),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
}

// A deleted host-proxy site must have its dev-server worker torn down, not just
// its registry entry removed, or the always-restart unit leaks. removeStale
// routes through siteops.TeardownSite, which calls the StopSiteWorkers hook.
func TestRemoveStale_tearsDownStaleHostProxyWorkers(t *testing.T) {
	isolateConfig(t)

	prev := siteops.StopSiteWorkers
	var stopped []string
	siteops.StopSiteWorkers = func(s *config.Site) { stopped = append(stopped, s.Name) }
	t.Cleanup(func() { siteops.StopSiteWorkers = prev })

	deletedDir := filepath.Join(t.TempDir(), "ghost")
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "ghost", Domains: []string{"ghost.test"}, Path: deletedDir, HostPort: 5197, HostCommand: "sleep 600"},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if !removeStale(&config.GlobalConfig{}) {
		t.Fatal("expected removeStale to report a removal")
	}
	if len(stopped) != 1 || stopped[0] != "ghost" {
		t.Errorf("removeStale must stop a stale host-proxy site's workers; stopped=%v", stopped)
	}
	if after, _ := config.LoadSites(); len(after.Sites) != 0 {
		t.Errorf("stale site should be removed from the registry; got %d sites", len(after.Sites))
	}
}

// A site whose directory is gone gets the same teardown an explicit unlink
// gives it. The sweep used to reimplement a subset of it, so a secured site
// left its cert pair behind, a custom-FPM site kept its quadlet, an open share
// stayed up and the recorded request timings outlived the site.
func TestRemoveStale_appliesTheFullUnlinkTeardown(t *testing.T) {
	isolateConfig(t)

	prevWorkers, prevShares := siteops.StopSiteWorkers, siteops.StopSiteShares
	var sharesStopped []string
	siteops.StopSiteWorkers = func(*config.Site) {}
	siteops.StopSiteShares = func(name string) { sharesStopped = append(sharesStopped, name) }
	t.Cleanup(func() {
		siteops.StopSiteWorkers = prevWorkers
		siteops.StopSiteShares = prevShares
	})

	siteCerts := filepath.Join(config.CertsDir(), "sites")
	for _, d := range []string{siteCerts, config.QuadletDir()} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	leftovers := []string{
		filepath.Join(siteCerts, "ghost.test.crt"),
		filepath.Join(siteCerts, "ghost.test.key"),
		filepath.Join(config.QuadletDir(), podman.CustomFPMContainerName("ghost")+".container"),
	}
	for _, f := range leftovers {
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := reqstats.SaveSnapshot([]reqstats.SiteStats{{Site: "ghost"}, {Site: "live"}}, config.RequestStatsFile()); err != nil {
		t.Fatal(err)
	}

	liveDir := t.TempDir()
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "ghost", Domains: []string{"ghost.test"}, Path: filepath.Join(t.TempDir(), "ghost"), Secured: true, Runtime: "fpm-custom"},
		{Name: "live", Domains: []string{"live.test"}, Path: liveDir},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if !removeStale(&config.GlobalConfig{}) {
		t.Fatal("expected removeStale to report a removal")
	}

	for _, f := range leftovers {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s survived the sweep", filepath.Base(f))
		}
	}
	if len(sharesStopped) != 1 || sharesStopped[0] != "ghost" {
		t.Errorf("the sweep must close the deleted site's shares; stopped=%v", sharesStopped)
	}
	if _, ok := reqstats.LoadSite(config.RequestStatsFile(), "ghost"); ok {
		t.Error("the stats file still carries the deleted site")
	}
	if _, ok := reqstats.LoadSite(config.RequestStatsFile(), "live"); !ok {
		t.Error("the sweep dropped an unrelated site's stats")
	}
}

func TestRemoveStale_removesDeletedNonParkedSite(t *testing.T) {
	isolateConfig(t)

	liveDir := t.TempDir()
	deletedDir := filepath.Join(t.TempDir(), "ghost")

	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "live", Domains: []string{"live.test"}, Path: liveDir},
		{Name: "ghost", Domains: []string{"ghost.test"}, Path: deletedDir},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if !removeStale(&config.GlobalConfig{}) {
		t.Fatal("expected removeStale to report a removal")
	}

	after, err := config.LoadSites()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(after.Sites))
	for _, s := range after.Sites {
		names = append(names, s.Name)
	}
	if len(names) != 1 || names[0] != "live" {
		t.Errorf("expected only [live] after sweep, got %v", names)
	}
}

func TestRemoveStale_keepsLiveSite(t *testing.T) {
	isolateConfig(t)

	liveDir := t.TempDir()
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "live", Domains: []string{"live.test"}, Path: liveDir},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if removeStale(&config.GlobalConfig{}) {
		t.Errorf("expected no removals when all site dirs exist")
	}
	after, _ := config.LoadSites()
	if len(after.Sites) != 1 {
		t.Errorf("expected live site preserved, got %d sites", len(after.Sites))
	}
}

func TestRemoveStale_skipsIgnoredSites(t *testing.T) {
	isolateConfig(t)

	// Ignored site with a deleted path should NOT be touched — the user has
	// intentionally parked it in the "ignored" state and the sweep shouldn't
	// reap it out from under them.
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "archived", Domains: []string{"archived.test"}, Path: "/var/empty/does-not-exist", Ignored: true},
	}}
	if err := config.SaveSites(reg); err != nil {
		t.Fatal(err)
	}

	if removeStale(&config.GlobalConfig{}) {
		t.Errorf("removeStale should not touch ignored sites")
	}
	after, _ := config.LoadSites()
	if len(after.Sites) != 1 {
		t.Errorf("ignored site should be preserved, got %d sites", len(after.Sites))
	}
}

// cleanupWorktreeVhosts runs after a worktree is removed and re-generates
// vhosts for surviving worktrees. It must NOT touch the surviving worktree's
// .env or kick off EnsureWorktreeDeps (which would trigger composer install
// and the JS install on every survivor on every removal).
func TestCleanupWorktreeVhosts_doesNotTouchSurvivorEnv(t *testing.T) {
	isolateConfig(t)

	mainSite := filepath.Join(t.TempDir(), "myapp")
	survivor := filepath.Join(t.TempDir(), "myapp-feat")
	for _, d := range []string{
		filepath.Join(mainSite, ".git", "worktrees", "feat"),
		survivor,
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(mainSite, ".env"), []byte("APP_URL=http://myapp.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtMeta := filepath.Join(mainSite, ".git", "worktrees", "feat")
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(survivor, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	site := &config.Site{
		Name:       "myapp",
		Domains:    []string{"myapp.test"},
		Path:       mainSite,
		PHPVersion: "8.3",
	}

	cleanupWorktreeVhosts(site)

	if _, err := os.Stat(filepath.Join(survivor, ".env")); err == nil {
		t.Fatal("cleanupWorktreeVhosts copied .env into survivor — EnsureWorktreeDeps must not run from cleanup path")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error on survivor .env: %v", err)
	}
}

// A removed worktree must lose its custom nginx override and backups, while a
// surviving worktree keeps its own.
func TestCleanupWorktreeVhosts_prunesRemovedWorktreeOverride(t *testing.T) {
	isolateConfig(t)

	mainSite := filepath.Join(t.TempDir(), "myapp")
	survivor := filepath.Join(t.TempDir(), "myapp-feat")
	for _, d := range []string{filepath.Join(mainSite, ".git", "worktrees", "feat"), survivor} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wtMeta := filepath.Join(mainSite, ".git", "worktrees", "feat")
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(survivor, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Both worktree vhosts exist in conf.d, but only "feat" still has a
	// worktree on disk; "gone" was removed.
	confD := config.NginxConfD()
	if err := os.MkdirAll(confD, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"feat.myapp.test", "gone.myapp.test"} {
		if err := os.WriteFile(filepath.Join(confD, d+".conf"), []byte("server {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	customD := config.NginxCustomD()
	bkpD := config.NginxCustomDBkp()
	for _, d := range []string{customD, bkpD} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	survivorOverride := filepath.Join(customD, "feat.myapp.test.conf")
	goneOverride := filepath.Join(customD, "gone.myapp.test.conf")
	goneBackup := filepath.Join(bkpD, "gone.myapp.test.conf.bkp.20260101-101010")
	for _, p := range []string{survivorOverride, goneOverride, goneBackup} {
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	site := &config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: mainSite, PHPVersion: "8.3"}
	cleanupWorktreeVhosts(site)

	if _, err := os.Stat(goneOverride); !os.IsNotExist(err) {
		t.Error("removed worktree's override should be pruned")
	}
	if _, err := os.Stat(goneBackup); !os.IsNotExist(err) {
		t.Error("removed worktree's backup should be pruned")
	}
	if _, err := os.Stat(survivorOverride); err != nil {
		t.Errorf("surviving worktree's override must be kept: %v", err)
	}
}

// HEAD writes (commit, checkout, rebase, rename) fire "changed" via
// fsnotify. Resurrecting host workers on every HEAD write resurrected
// user stops on every commit — issue #375 (Bruno's Vite).
// cleanupWorktreeVhosts must not delete the custom override of a separately
// registered site whose primary is a subdomain of the cleaned site.
func TestCleanupWorktreeVhosts_keepsSiblingSubdomainSiteOverride(t *testing.T) {
	isolateConfig(t)

	mainSite := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(filepath.Join(mainSite, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "app", Domains: []string{"app.test"}, Path: mainSite, PHPVersion: "8.3"}); err != nil {
		t.Fatal(err)
	}
	// A real second site whose primary is admin.app.test.
	if err := config.AddSite(config.Site{Name: "admin", Domains: []string{"admin.app.test"}, Path: t.TempDir(), PHPVersion: "8.3"}); err != nil {
		t.Fatal(err)
	}

	confD := config.NginxConfD()
	customD := config.NginxCustomD()
	for _, d := range []string{confD, customD} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// admin.app.test has a vhost (matches app's suffix scan) and an override.
	if err := os.WriteFile(filepath.Join(confD, "admin.app.test.conf"), []byte("server {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	siblingOverride := filepath.Join(customD, "admin.app.test.conf")
	if err := os.WriteFile(siblingOverride, []byte("# sibling\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanupWorktreeVhosts(&config.Site{Name: "app", Domains: []string{"app.test"}, Path: mainSite, PHPVersion: "8.3"})

	if _, err := os.Stat(siblingOverride); err != nil {
		t.Errorf("sibling site's override must survive cleanup of app: %v", err)
	}
}

// Inheritance of the main override must fire only on genuine creation; on a
// "changed" event (commit/checkout) re-seeding would resurrect an override the
// user deliberately reset.
func TestShouldInheritNginxOnSync(t *testing.T) {
	cases := map[string]bool{"added": true, "changed": false, "": false, "removed": false}
	for action, want := range cases {
		if got := shouldInheritNginxOnSync(action); got != want {
			t.Errorf("shouldInheritNginxOnSync(%q) = %v, want %v", action, got, want)
		}
	}
}

// The boot scan provisions worktrees, and a composer install there can take
// longer than the Type=notify unit's start timeout. Readiness must be signalled
// without waiting for it, or systemd terminates the process before it is ever
// ready and the restart begins the same install again, forever.
func TestNotifyReadyThenScan_readinessDoesNotWaitOnTheScan(t *testing.T) {
	release := make(chan struct{})
	// A scan run synchronously would block on release forever, so it is let go
	// on a timer as well and the assertions below report the ordering.
	releaseScan := sync.OnceFunc(func() { close(release) })
	time.AfterFunc(5*time.Second, releaseScan)

	var ready atomic.Bool
	scanDone := make(chan struct{})
	notifyReadyThenScan(
		func() { ready.Store(true) },
		func() { <-release; close(scanDone) },
	)

	if !ready.Load() {
		t.Error("readiness was not signalled before the watcher moved on")
	}
	select {
	case <-scanDone:
		t.Error("readiness waited for the boot scan to finish")
	default:
	}

	releaseScan()
	<-scanDone
}

// A worktree's subdomain only needs its vhost, so the boot scan must write
// every one of them before it starts installing anything. Serving behind the
// installs meant the last worktree of a large repo kept 404ing for as long as
// all the earlier composer/npm runs took.
func TestScanWorktrees_writesVhostBeforeTheDependencyInstall(t *testing.T) {
	isolateConfig(t)

	mainRepo := filepath.Join(t.TempDir(), "myapp")
	worktree := filepath.Join(t.TempDir(), "myapp-feat")
	wtMeta := filepath.Join(mainRepo, ".git", "worktrees", "feat")
	for _, d := range []string{wtMeta, worktree} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(mainRepo, ".env"), []byte("APP_URL=http://myapp.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(worktree, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "myapp", Domains: []string{"myapp.test"}, Path: mainRepo, PHPVersion: "8.3",
	}); err != nil {
		t.Fatal(err)
	}

	provision, generated := scanWorktrees()

	if !generated {
		t.Fatal("expected the scan to report a generated vhost")
	}
	vhost := filepath.Join(config.NginxConfD(), "feat.myapp.test.conf")
	if _, err := os.Stat(vhost); err != nil {
		t.Errorf("worktree vhost must exist as soon as the scan returns: %v", err)
	}
	wtEnv := filepath.Join(worktree, ".env")
	if _, err := os.Stat(wtEnv); !os.IsNotExist(err) {
		t.Error("the dependency seed must be deferred, not run before the vhost is written")
	}
	if len(provision) != 1 {
		t.Fatalf("expected one deferred provisioning job, got %d", len(provision))
	}

	runProvisioning(provision, 1)

	if _, err := os.Stat(wtEnv); err != nil {
		t.Errorf("deferred provisioning must still seed the worktree: %v", err)
	}
}

// A worktree that pins its own PHP version in .lerd.yaml must be served by that
// version's FPM container. The boot scan wrote the parent's instead, so every
// watcher restart pointed such a worktree at the wrong container until some
// later pass corrected it.
func TestScanWorktrees_vhostHonoursTheWorktreePHPPin(t *testing.T) {
	isolateConfig(t)

	mainRepo := filepath.Join(t.TempDir(), "myapp")
	worktree := filepath.Join(t.TempDir(), "myapp-feat")
	wtMeta := filepath.Join(mainRepo, ".git", "worktrees", "feat")
	for _, d := range []string{wtMeta, worktree} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(worktree, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The branch is on 8.3 while the parent site stays on 8.4.
	if err := config.SetWorktreePHPVersion(worktree, "8.3"); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{
		Name: "myapp", Domains: []string{"myapp.test"}, Path: mainRepo, PHPVersion: "8.4",
	}); err != nil {
		t.Fatal(err)
	}

	scanWorktrees()

	vhost, err := os.ReadFile(filepath.Join(config.NginxConfD(), "feat.myapp.test.conf"))
	if err != nil {
		t.Fatalf("reading the generated worktree vhost: %v", err)
	}
	if !strings.Contains(string(vhost), "lerd-php83-fpm") {
		t.Errorf("worktree vhost must route to the pinned 8.3 container, got:\n%s", vhost)
	}
	if strings.Contains(string(vhost), "lerd-php84-fpm") {
		t.Error("worktree vhost fell back to the parent site's PHP container")
	}
}

// The boot scan runs unattended while the machine is being used for something
// else, so it never claims every core: two are left for the daemons, the
// containers serving the other sites, and the user's own work.
func TestProvisionSlots(t *testing.T) {
	cases := map[int]int{1: 1, 2: 1, 3: 1, 4: 2, 5: 3, 6: 4, 8: 4, 16: 4, 64: 4}
	for numCPU, want := range cases {
		if got := provisionSlots(numCPU); got != want {
			t.Errorf("provisionSlots(%d) = %d, want %d", numCPU, got, want)
		}
	}
	if got := provisionSlots(0); got != 1 {
		t.Errorf("provisionSlots must never return less than one slot, got %d", got)
	}
}

// Worktrees are independent of each other, so the deferred half of the boot
// scan runs them concurrently — bounded, since each one is a composer and a
// JS install and a repo with dozens of worktrees would otherwise fork all of
// them at once.
func TestRunProvisioning_runsEveryJobWithinTheConcurrencyLimit(t *testing.T) {
	const jobs, limit = 12, 3

	var live, peak, done atomic.Int64
	entered := make(chan struct{}, jobs)
	release := make(chan struct{})
	work := make([]func(), 0, jobs)
	for range jobs {
		work = append(work, func() {
			n := live.Add(1)
			for p := peak.Load(); n > p; p = peak.Load() {
				if peak.CompareAndSwap(p, n) {
					break
				}
			}
			entered <- struct{}{}
			// Hold every slot open until the limit is reached, so it is the
			// limit that caps the count and not jobs finishing before their
			// peers have started.
			<-release
			live.Add(-1)
			done.Add(1)
		})
	}

	finished := make(chan struct{})
	go func() { runProvisioning(work, limit); close(finished) }()

	for range limit {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d slots ever filled", peak.Load(), limit)
		}
	}
	close(release)
	<-finished

	if got := peak.Load(); got > limit {
		t.Errorf("ran %d jobs at once, limit is %d", got, limit)
	}
	if got := done.Load(); got != jobs {
		t.Errorf("ran %d of %d jobs", got, jobs)
	}
}

func TestShouldAutoStartWorkersOnSync(t *testing.T) {
	cases := map[string]bool{
		"added":   true,
		"changed": false,
		"":        false,
		"removed": false,
	}
	for action, want := range cases {
		if got := shouldAutoStartWorkersOnSync(action); got != want {
			t.Errorf("shouldAutoStartWorkersOnSync(%q) = %v, want %v", action, got, want)
		}
	}
}

func TestPrintDNSDiagnostic_WarnStepShowsHint(t *testing.T) {
	// Regression for the field-report scenario: the legacy-host-resolver
	// path emits a WARN step whose Hint explains how to switch back to
	// lerd-managed DNS. The renderer used to print hints only on FAIL
	// steps, silently swallowing this guidance.
	diag := dns.Diagnostic{
		TLD:          "test",
		FirstFailure: -1,
		Steps: []dns.Step{{
			Name:   "lerd-dns container",
			Status: dns.StepWarn,
			Detail: "not running; a host-side resolver on :5300 is answering .test with 127.0.0.1",
			Hint:   "lerd is not managing DNS here. Stop your host resolver and run `lerd start` to switch.",
		}},
	}
	var buf bytes.Buffer
	printDNSDiagnostic(&buf, diag)
	out := buf.String()

	for _, want := range []string{
		"DNS is working",
		"! lerd-dns container",
		"host-side resolver on :5300",
		"hint:",
		"Stop your host resolver",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestPrintDNSDiagnostic_FailStepShowsHint(t *testing.T) {
	diag := dns.Diagnostic{
		TLD:          "test",
		FirstFailure: 0,
		Steps: []dns.Step{{
			Name:   "lerd-dns container",
			Status: dns.StepFail,
			Detail: "not running",
			Hint:   "lerd start  (or check podman logs lerd-dns)",
		}},
	}
	var buf bytes.Buffer
	printDNSDiagnostic(&buf, diag)
	out := buf.String()
	for _, want := range []string{"DNS is NOT working", "✗ lerd-dns container", "hint: lerd start"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestPrintDNSDiagnostic_OKStepHasNoHintLine(t *testing.T) {
	diag := dns.Diagnostic{
		TLD:          "test",
		FirstFailure: -1,
		Steps: []dns.Step{{
			Name:   "lerd-dns container",
			Status: dns.StepOK,
			Detail: "running",
			Hint:   "this hint should be ignored on OK steps",
		}},
	}
	var buf bytes.Buffer
	printDNSDiagnostic(&buf, diag)
	out := buf.String()
	if strings.Contains(out, "hint:") {
		t.Errorf("OK step should not print a hint line, got:\n%s", out)
	}
}

// TestShutdownOnSignal_TearsDownThenUnblocksWatch pins the logout path: the
// signal runs the teardown, and only then cancels the watch loop so the
// process exits on its own instead of being killed mid-shutdown.
func TestShutdownOnSignal_TearsDownThenUnblocksWatch(t *testing.T) {
	isolateConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	quitDone := make(chan struct{})
	quit := func() error {
		select {
		case <-ctx.Done():
			t.Error("watch loop was cancelled before the teardown finished")
		default:
		}
		close(quitDone)
		return nil
	}

	sigs <- syscall.SIGTERM
	shutdownOnSignal(sigs, quit, cancel)

	select {
	case <-quitDone:
	default:
		t.Fatal("teardown never ran")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("watch loop was never cancelled, the watcher would hang until SIGKILL")
	}
}

// TestShutdownOnSignal_NoTeardownPlatformJustExits pins the platform gate. Where
// nothing outlives the session there is nothing to protect, and running the
// teardown would turn an ordinary `systemctl --user stop lerd-watcher` into a
// stop of every container, lerd-ui and lerd-dns.
func TestShutdownOnSignal_NoTeardownPlatformJustExits(t *testing.T) {
	isolateConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGTERM
	shutdownOnSignal(sigs, nil, cancel)

	select {
	case <-ctx.Done():
	default:
		t.Error("the watcher must still exit when the platform runs no teardown")
	}
}

// TestShutdownOnSignal_NoTeardownPlatformKeepsTheMarker pins that the early exit
// does not eat a marker it never earned, exactly as the Ctrl-C exit does not.
func TestShutdownOnSignal_NoTeardownPlatformKeepsTheMarker(t *testing.T) {
	isolateConfig(t)
	if err := config.MarkWatcherManagedStop(); err != nil {
		t.Fatalf("MarkWatcherManagedStop: %v", err)
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGTERM
	shutdownOnSignal(sigs, nil, cancel)

	if !config.ConsumeWatcherManagedStop() {
		t.Error("an exit with no teardown must leave the marker for the real stop")
	}
}

// TestShutdownOnSignal_QuitErrorStillExits pins that a failed teardown does not
// leave the watcher blocked: launchd would SIGKILL it after the exit timeout.
func TestShutdownOnSignal_QuitErrorStillExits(t *testing.T) {
	isolateConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGTERM
	shutdownOnSignal(sigs, func() error { return errors.New("boom") }, cancel)

	select {
	case <-ctx.Done():
	default:
		t.Error("watch loop must be cancelled even when the teardown fails")
	}
}

// TestShutdownOnSignal_ManagedStopSkipsTeardown pins the guard that keeps
// `lerd install`, `lerd update` and `lerd quit` from tearing the environment
// down. They all stop the watcher, and launchd delivers that as the same
// SIGTERM a logout does; without the marker an install would stop every
// container and the Podman Machine VM halfway through.
func TestShutdownOnSignal_ManagedStopSkipsTeardown(t *testing.T) {
	isolateConfig(t)
	if err := config.MarkWatcherManagedStop(); err != nil {
		t.Fatalf("MarkWatcherManagedStop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGTERM
	tore := false
	shutdownOnSignal(sigs, func() error { tore = true; return nil }, cancel)

	if tore {
		t.Error("a lerd-managed stop must not run the shutdown teardown")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("the watcher must still exit on a managed stop")
	}
}

// TestShutdownOnSignal_ManagedStopIsConsumedOnce pins that the marker does not
// linger: a managed restart followed by a real logout must still tear down.
func TestShutdownOnSignal_ManagedStopIsConsumedOnce(t *testing.T) {
	isolateConfig(t)
	if err := config.MarkWatcherManagedStop(); err != nil {
		t.Fatalf("MarkWatcherManagedStop: %v", err)
	}

	_, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	first := make(chan os.Signal, 1)
	first <- syscall.SIGTERM
	shutdownOnSignal(first, func() error { return nil }, cancel1)

	_, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	second := make(chan os.Signal, 1)
	second <- syscall.SIGTERM
	tore := false
	shutdownOnSignal(second, func() error { tore = true; return nil }, cancel2)

	if !tore {
		t.Error("a stale marker suppressed a real logout teardown")
	}
}

// TestShutdownOnSignal_InterruptSkipsTeardown pins that Ctrl-C on a hand-run
// `lerd watch` does not tear the environment down. launchd and systemd only
// ever signal a shutdown with SIGTERM, so a SIGINT means a person at a
// terminal who wants their shell back, not their containers stopped.
func TestShutdownOnSignal_InterruptSkipsTeardown(t *testing.T) {
	isolateConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGINT
	tore := false
	shutdownOnSignal(sigs, func() error { tore = true; return nil }, cancel)

	if tore {
		t.Error("Ctrl-C must not stop every container and the Podman Machine VM")
	}
	select {
	case <-ctx.Done():
	default:
		t.Error("the watcher must still exit on Ctrl-C")
	}
}

// TestShutdownOnSignal_InterruptLeavesTheMarkerAlone pins that the SIGINT exit
// does not consume a managed-stop marker it never earned. Eating one here would
// leave the SIGTERM that follows reading as a logout mid-install.
func TestShutdownOnSignal_InterruptLeavesTheMarkerAlone(t *testing.T) {
	isolateConfig(t)
	if err := config.MarkWatcherManagedStop(); err != nil {
		t.Fatalf("MarkWatcherManagedStop: %v", err)
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	sigs <- syscall.SIGINT
	shutdownOnSignal(sigs, func() error { return nil }, cancel)

	if !config.ConsumeWatcherManagedStop() {
		t.Error("a SIGINT exit must leave the managed-stop marker for the real stop")
	}
}

func TestAcceptDetectedPHP(t *testing.T) {
	installed := func(v string) bool { return v == "8.5" || v == "8.4" }
	var out bytes.Buffer
	if !acceptDetectedPHP("demo", "8.5", "8.4", installed, &out) {
		t.Error("an installed version must be accepted")
	}
	if acceptDetectedPHP("demo", "8.5", "8.3", installed, &out) {
		t.Error("a version with no FPM container must be refused")
	}
	if !strings.Contains(out.String(), "PHP 8.3") || !strings.Contains(out.String(), "still serving 8.5") {
		t.Errorf("warning should name both versions, got %q", out.String())
	}
}

func TestAnnounceSiteFilesChangedReachesLerdUI(t *testing.T) {
	orig := notifyUI
	defer func() { notifyUI = orig }()
	var called bool
	notifyUI = func(string) { called = true }

	announceSiteFilesChanged()

	if !called {
		t.Error("a site file change must be posted to lerd-ui, the watcher's event bus stays in its own process")
	}
}
