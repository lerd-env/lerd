package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// portFixture registers one host-proxy site on port 43000 and fakes which ports
// are taken. moves counts the vhost rewrites.
func portFixture(t *testing.T, movePort bool, taken ...int) (site *config.Site, moves *int) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	def := "name: acmejs\nlabel: AcmeJS\ndev_server:\n  command: npm run acme\n"
	if movePort {
		def += "  move_port: true\n"
	}
	if err := os.MkdirAll(config.FrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.FrameworksDir(), "acmejs.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	s := config.Site{Name: "api", Domains: []string{"api.test"}, Path: t.TempDir(), Framework: "acmejs", HostPort: 43000, HostCommand: "npm run acme"}
	if err := config.AddSite(s); err != nil {
		t.Fatal(err)
	}
	pf, pw, pr, pp, poll := devServerPortFree, devServerPortWait, devServerRunning, devServerRepoint, hostProxyStopPoll
	t.Cleanup(func() {
		devServerPortFree, devServerPortWait, devServerRunning, devServerRepoint, hostProxyStopPoll = pf, pw, pr, pp, poll
	})
	moves = new(int)
	devServerPortFree = func(p int) bool {
		for _, n := range taken {
			if n == p {
				return false
			}
		}
		return true
	}
	devServerPortWait, hostProxyStopPoll = 5*time.Millisecond, time.Millisecond
	devServerRunning = func(string) bool { return false }
	devServerRepoint = func(*config.Site) error { *moves++; return nil }
	return &s, moves
}

func proxyOn(port int) *config.ProxyConfig {
	return &config.ProxyConfig{Command: "npm run acme", Port: port}
}

func TestSettleDevServerPort_movesOffATakenPort(t *testing.T) {
	site, moves := portFixture(t, true, 43000)
	proxy := proxyOn(43000)
	if !settleDevServerPort(site, proxy) {
		t.Fatal("the port did not move")
	}
	if site.HostPort <= 43000 || site.HostPortMovedFrom != 43000 || *moves != 1 {
		t.Fatalf("port %d moved from %d, %d vhost rewrites", site.HostPort, site.HostPortMovedFrom, *moves)
	}
	saved, err := config.FindSite("api")
	if err != nil || saved.HostPort != site.HostPort || saved.HostPortMovedFrom != 43000 {
		t.Errorf("registry = %+v (%v), want the moved port", saved, err)
	}
	if live := liveProxy(*site, proxy); live.Port != site.HostPort {
		t.Errorf("the command gets port %d, nginx %d", live.Port, site.HostPort)
	}
	if proxy.Port != 43000 {
		t.Error("the committed proxy block was changed")
	}

	// Still taken: the next start keeps the moved port rather than hopping on.
	was := site.HostPort
	if settleDevServerPort(site, proxy) || site.HostPort != was {
		t.Errorf("a second start moved again, to %d", site.HostPort)
	}

	devServerPortFree = func(int) bool { return true }
	if !settleDevServerPort(site, proxy) || site.HostPort != 43000 || site.HostPortMovedFrom != 0 {
		t.Errorf("port %d from %d, want the declared port back once free", site.HostPort, site.HostPortMovedFrom)
	}
}

func TestSettleDevServerPort_leavesThePortAlone(t *testing.T) {
	cases := map[string]struct {
		movePort bool
		taken    []int
		proxy    *config.ProxyConfig
		running  bool
	}{
		"free":                       {true, nil, proxyOn(43000), false},
		"framework does not allow":   {false, []int{43000}, proxyOn(43000), false},
		"already running":            {true, []int{43000}, proxyOn(43000), true},
		"command names the port":     {true, []int{43000}, &config.ProxyConfig{Command: "npm run acme -p 43000", Port: 43000}, false},
		"proxy-only, nothing to run": {true, []int{43000}, &config.ProxyConfig{Port: 43000}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			site, moves := portFixture(t, c.movePort, c.taken...)
			devServerRunning = func(string) bool { return c.running }
			if settleDevServerPort(site, c.proxy) || site.HostPort != 43000 || *moves != 0 {
				t.Errorf("port %d, %d vhost rewrites, want it left on 43000", site.HostPort, *moves)
			}
		})
	}
}

func TestSettleDevServerPort_waitsForItsOwnOldProcess(t *testing.T) {
	site, _ := portFixture(t, true)
	asked := 0
	devServerPortWait = time.Minute
	devServerPortFree = func(int) bool { asked++; return asked > 2 }
	if settleDevServerPort(site, proxyOn(43000)) {
		t.Errorf("moved to %d although the port came free", site.HostPort)
	}
}

// A sleeping site holds its port too: nothing listens, but it wakes there.
func TestSettleDevServerPort_avoidsAnotherSitesPort(t *testing.T) {
	site, _ := portFixture(t, true, 43000)
	for _, p := range []int{43001, 43002} {
		name := "other" + string(rune('a'+p-43001))
		if err := config.AddSite(config.Site{Name: name, Domains: []string{name + ".test"}, Path: t.TempDir(), HostPort: p}); err != nil {
			t.Fatal(err)
		}
	}
	settleDevServerPort(site, proxyOn(43000))
	if site.HostPort <= 43002 {
		t.Errorf("moved to %d, a port another site has", site.HostPort)
	}
}

// Nothing listens on a sleeping site's port, but it is still that site's.
func TestSettleDevServerPort_leavesASleepingSiteItsPort(t *testing.T) {
	site, _ := portFixture(t, true)
	if err := config.AddSite(config.Site{Name: "sleeper", Domains: []string{"sleeper.test"}, Path: t.TempDir(), HostPort: 43000}); err != nil {
		t.Fatal(err)
	}
	if !settleDevServerPort(site, proxyOn(43000)) || site.HostPort == 43000 {
		t.Error("started on the port of a sleeping site")
	}
}

// A stopped worktree's port is only in the registry.
func TestSettleDevServerPort_leavesAWorktreeItsPort(t *testing.T) {
	site, _ := portFixture(t, true)
	other := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), HostPort: 44000, WorktreeHostPorts: map[string]int{"shop-feature": 43000}}
	if err := config.AddSite(other); err != nil {
		t.Fatal(err)
	}
	if !settleDevServerPort(site, proxyOn(43000)) || site.HostPort == 43000 {
		t.Error("took the port of another site's worktree")
	}
}

func TestSettleDevServerPort_leavesItsOwnWorktreeItsPort(t *testing.T) {
	site, _ := portFixture(t, true, 43000)
	saved, err := config.FindSite("api")
	if err != nil {
		t.Fatal(err)
	}
	saved.WorktreeHostPorts = map[string]int{"api-feature": 43001}
	if err := config.AddSite(*saved); err != nil {
		t.Fatal(err)
	}
	if !settleDevServerPort(site, proxyOn(43000)) || site.HostPort == 43001 {
		t.Errorf("moved to %d, its own worktree's port", site.HostPort)
	}
}

// failingStart is a dev server that stops and then cannot start.
type failingStart struct{ devServerLifecycle }

func (f *failingStart) Start(unit string) error {
	f.ops = append(f.ops, "start "+unit)
	return errors.New("no such unit")
}

func movableRestart(t *testing.T) {
	t.Helper()
	site, _ := portFixture(t, true)
	if err := os.WriteFile(filepath.Join(site.Path, ".lerd.yaml"), []byte("proxy:\n  command: npm run acme\n  port: 43000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := podman.DaemonReloadFn
	podman.DaemonReloadFn = func() error { return nil }
	t.Cleanup(func() { podman.UnitLifecycle, podman.DaemonReloadFn = nil, prev })
}

func TestRestartSite_reportsAMovableServerThatDoesNotStart(t *testing.T) {
	movableRestart(t)
	podman.UnitLifecycle = &failingStart{}
	if err := RestartSite("api"); err == nil || !strings.Contains(err.Error(), "no such unit") {
		t.Errorf("err = %v, want the start failure", err)
	}
}

func TestRestartSite_refusedCommandLeavesTheServerRunning(t *testing.T) {
	movableRestart(t)
	life := &failingStart{}
	podman.UnitLifecycle = life
	if err := os.WriteFile(config.GlobalConfigFile(), []byte("host_proxy:\n  disabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestartSite("api"); err == nil || len(life.ops) != 0 {
		t.Errorf("err = %v, ops = %v, want a refusal that stops nothing", err, life.ops)
	}
}

// The idle engine and a restart can start one server at the same time, each
// holding its own copy of the site.
func TestSettleDevServerPort_followsAMoveByAnotherProcess(t *testing.T) {
	site, moves := portFixture(t, true, 43000)
	stale := *site
	settleDevServerPort(site, proxyOn(43000))
	settleDevServerPort(&stale, proxyOn(43000))
	if stale.HostPort != site.HostPort {
		t.Errorf("moved to %d and then to %d", site.HostPort, stale.HostPort)
	}
	if *moves != 2 {
		t.Errorf("%d vhost rewrites, want the stale copy's vhost corrected too", *moves)
	}
	devServerRunning = func(string) bool { return true }
	stale.HostPort, stale.HostPortMovedFrom = 43000, 0
	settleDevServerPort(&stale, proxyOn(43000))
	if stale.HostPort != site.HostPort {
		t.Errorf("a running server's copy says %d, want %d", stale.HostPort, site.HostPort)
	}
}

func TestHostProxyPassRe(t *testing.T) {
	for conf, want := range map[string]bool{
		"proxy_pass http://169.254.1.2:3000;":               true,
		"proxy_pass https://host.containers.internal:3000;": true,
		"proxy_pass http://$backend:7073/api/idle/waking;":  false,
		"root /paused; try_files /paused.html =404;":        false,
	} {
		if got := hostProxyPassRe.MatchString(conf); got != want {
			t.Errorf("%q = %v, want %v", conf, got, want)
		}
	}
}

// A .lerd.yaml can gain a proxy block before the site is linked again.
func TestSettleDevServerPort_neverTurnsASiteIntoAProxy(t *testing.T) {
	site, _ := portFixture(t, true)
	if err := config.SetSiteHostPort("api", 0, 0); err != nil {
		t.Fatal(err)
	}
	site.HostPort = 0
	if settleDevServerPort(site, proxyOn(43000)) || site.HostPort != 0 {
		t.Errorf("port = %d, want the site left as it is linked", site.HostPort)
	}
}

// A port edited in .lerd.yaml takes nginx with it on the next start.
func TestSettleDevServerPort_followsAnEditedPort(t *testing.T) {
	site, moves := portFixture(t, false)
	if !settleDevServerPort(site, proxyOn(43500)) || site.HostPort != 43500 || *moves != 1 {
		t.Errorf("port %d, %d vhost rewrites, want 43500 and one", site.HostPort, *moves)
	}
}

func TestProxyConfigFor_carriesTheFrameworksVariables(t *testing.T) {
	p := proxyConfigFor(&config.FrameworkDevServer{PortEnvKey: "BIND_PORT", HostEnvKey: "BIND_HOST"}, "run", 4000)
	if p.PortEnvKey != "BIND_PORT" || p.HostEnvKey != "BIND_HOST" || p.Port != 4000 {
		t.Errorf("proxy = %+v", p)
	}
	if p := proxyConfigFor(nil, "run", 4000); p.PortEnvKey != "" {
		t.Errorf("proxy = %+v", p)
	}
}
