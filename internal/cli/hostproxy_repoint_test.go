package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/podman"
)

const (
	lanGateway   = "192.168.1.10"
	lerd0Gateway = "192.0.2.1"
)

// repointFixture is a host-proxy site whose hosts file is on lerd0 while its vhost
// and, when unitHost is set, its dev-server unit carry the given addresses. The
// command approved at link is "npm run dev"; yamlCommand is what .lerd.yaml says.
type repointFixture struct {
	vhost, unit string
	life        *devServerLifecycle
	reloads     int
}

func newRepointFixture(t *testing.T, vhostHost, unitHost, yamlCommand string) *repointFixture {
	t.Helper()
	if platform.Current.UsesMachineVM {
		t.Skip("the vhost upstream is a hostname under a machine VM")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	pinHostGatewayBindIP(t, lerd0Gateway)

	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(config.ContainerHostsFile(), "127.0.0.1 localhost\n"+lerd0Gateway+" host.containers.internal host.docker.internal\n")

	dir := t.TempDir()
	write(filepath.Join(dir, ".lerd.yaml"), "proxy:\n  command: "+yamlCommand+"\n  port: 3000\n")
	if err := config.AddSite(config.Site{Name: "app", Domains: []string{"app.test"}, Path: dir, HostPort: 3000, HostCommand: "npm run dev"}); err != nil {
		t.Fatal(err)
	}

	f := &repointFixture{
		vhost: filepath.Join(config.NginxConfD(), "app.test.conf"),
		unit:  filepath.Join(config.SystemdUserDir(), hostProxyWorkerUnit("app")+".service"),
		life:  &devServerLifecycle{},
	}
	write(f.vhost, "server {\n    location / {\n        proxy_pass http://"+vhostHost+":3000;\n    }\n}\n")
	if unitHost != "" {
		write(f.unit, "[Service]\nExecStart=/bin/sh -c 'env PORT=3000 HOST="+unitHost+" npm run dev'\n")
	}

	podman.UnitLifecycle = f.life
	prevReload, prevDaemon := hostProxyNginxReload, podman.DaemonReloadFn
	prevProbe, prevPoll := devServerPortInUse, hostProxyStopPoll
	hostProxyNginxReload = func() error { f.reloads++; return nil }
	podman.DaemonReloadFn = func() error { return nil }
	devServerPortInUse = func(int) bool { return false }
	hostProxyStopPoll = time.Millisecond
	t.Cleanup(func() {
		podman.UnitLifecycle = nil
		hostProxyNginxReload, podman.DaemonReloadFn = prevReload, prevDaemon
		devServerPortInUse, hostProxyStopPoll = prevProbe, prevPoll
	})
	return f
}

func (f *repointFixture) read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The upgrade case: lerd start moved the hosts file onto lerd0 and nothing told
// the watcher, so a vhost left on the LAN address is repointed on the next tick.
func TestRepointStaleHostProxySites_repointsAVhostLeftOnTheOldAddress(t *testing.T) {
	f := newRepointFixture(t, lanGateway, "", "npm run dev")

	if !RepointStaleHostProxySites() {
		t.Fatal("a vhost on the old address should be repointed")
	}
	if got := f.read(t, f.vhost); !strings.Contains(got, "http://"+lerd0Gateway+":3000") {
		t.Errorf("vhost not repointed at the hosts-file gateway:\n%s", got)
	}
	if f.reloads != 1 {
		t.Errorf("nginx reloaded %d times, want 1", f.reloads)
	}
	if RepointStaleHostProxySites() {
		t.Error("a second pass found work left over")
	}
}

func TestRepointStaleHostProxySites_rebindsAnApprovedRunningServer(t *testing.T) {
	f := newRepointFixture(t, lanGateway, lanGateway, "npm run dev")

	RepointStaleHostProxySites()

	if want := "stop lerd-app-app,start lerd-app-app"; strings.Join(f.life.ops, ",") != want {
		t.Errorf("ops = %v, want %s", f.life.ops, want)
	}
	if got := f.read(t, f.unit); !strings.Contains(got, "HOST="+lerd0Gateway+" ") {
		t.Errorf("unit not rebound to the hosts-file gateway:\n%s", got)
	}
	f.life.stopped = false
	if RepointStaleHostProxySites() || len(f.life.ops) != 2 {
		t.Errorf("a second pass found work left over, ops = %v", f.life.ops)
	}
}

// A command edited in .lerd.yaml since link waits for a relink: the vhost still
// moves, but the server is neither rewritten nor restarted, now or on later ticks.
func TestRepointStaleHostProxySites_leavesAChangedCommandAlone(t *testing.T) {
	f := newRepointFixture(t, lanGateway, lanGateway, "npm run other")
	unitBefore := f.read(t, f.unit)

	RepointStaleHostProxySites()

	if len(f.life.ops) != 0 {
		t.Errorf("a changed command was restarted: %v", f.life.ops)
	}
	if f.read(t, f.unit) != unitBefore {
		t.Error("the unit of a changed command was rewritten")
	}
	if got := f.read(t, f.vhost); !strings.Contains(got, "http://"+lerd0Gateway+":3000") {
		t.Errorf("vhost not repointed:\n%s", got)
	}
	if RepointStaleHostProxySites() {
		t.Error("an unapproved server kept the pass busy on every tick")
	}
}

func TestRepointStaleHostProxySites_leavesAMatchingSiteAlone(t *testing.T) {
	f := newRepointFixture(t, lerd0Gateway, lerd0Gateway, "npm run dev")
	vhostBefore := f.read(t, f.vhost)

	if RepointStaleHostProxySites() {
		t.Error("a site already on the gateway was repointed")
	}
	if f.reloads != 0 || len(f.life.ops) != 0 || f.read(t, f.vhost) != vhostBefore {
		t.Errorf("a matching site was touched: reloads=%d ops=%v", f.reloads, f.life.ops)
	}
}

// When a network change already fired the gateway callback this tick, the pass
// that follows it finds nothing left to repoint.
func TestRepointStaleHostProxySites_findsNothingAfterTheGatewayCallback(t *testing.T) {
	f := newRepointFixture(t, lanGateway, lanGateway, "npm run dev")

	RegenerateHostProxyVhostsOnGatewayChange()
	f.life.stopped = false
	ops, reloads := len(f.life.ops), f.reloads

	if RepointStaleHostProxySites() || len(f.life.ops) != ops || f.reloads != reloads {
		t.Errorf("the pass redid the callback's work: ops=%v reloads=%d", f.life.ops, f.reloads)
	}
}
