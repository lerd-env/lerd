package podman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func TestFirstField(t *testing.T) {
	cases := map[string]string{
		"169.254.1.2     host.containers.internal host.docker.internal\n": "169.254.1.2",
		"  10.0.2.2 host.containers.internal\n":                           "10.0.2.2",
		"":                                                                "",
		"\n\n":                                                            "",
		"only-one-tok":                                                    "only-one-tok",
	}
	for in, want := range cases {
		if got := firstField(in); got != want {
			t.Errorf("firstField(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseNginxIP(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "site entry wins",
			in: "127.0.0.1 localhost\n::1 localhost\n" +
				"169.254.1.2 host.containers.internal host.docker.internal\n" +
				"10.89.7.11 blog.test\n10.89.7.11 shop.test\n",
			want: "10.89.7.11",
		},
		{
			// No sites linked yet: nothing to compare against, so the watcher
			// must not treat the gateway or loopback line as an nginx IP.
			name: "no site entries",
			in: "127.0.0.1 localhost\n::1 localhost\n" +
				"169.254.1.2 host.containers.internal host.docker.internal\n",
			want: "",
		},
		{
			name: "gateway aliased to a bare host.docker.internal",
			in:   "127.0.0.1 localhost\n10.0.2.2 host.docker.internal\n10.89.7.11 blog.test\n",
			want: "10.89.7.11",
		},
		{
			name: "blank lines and trailing whitespace",
			in:   "\n\n  10.89.7.11   blog.test  \n",
			want: "10.89.7.11",
		},
		{name: "empty file", in: "", want: ""},
		{name: "malformed single-token lines", in: "garbage\n127.0.0.1\n", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseNginxIP([]byte(c.in)); got != c.want {
				t.Errorf("parseNginxIP() = %q, want %q", got, c.want)
			}
		})
	}
}

// The reader and the writer have to agree, or the watcher compares a value
// it can never produce and rewrites the file on every tick.
func TestParseNginxIP_RoundTripsRenderContainerHosts(t *testing.T) {
	reg := &config.SiteRegistry{Sites: []config.Site{{Domains: []string{"blog.test"}}}}
	rendered := renderContainerHosts(reg, "169.254.1.2", "10.89.7.11")
	if got := parseNginxIP([]byte(rendered)); got != "10.89.7.11" {
		t.Errorf("parseNginxIP(rendered) = %q, want %q", got, "10.89.7.11")
	}
}

func TestReadNginxIPFromFile_MissingFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := ReadNginxIPFromFile(); got != "" {
		t.Errorf("ReadNginxIPFromFile() with no file = %q, want %q", got, "")
	}
}

func TestRenderContainerHosts_EmptyRegistry(t *testing.T) {
	got := renderContainerHosts(&config.SiteRegistry{}, "169.254.1.2", "10.89.0.2")
	want := "127.0.0.1 localhost\n" +
		"::1 localhost\n" +
		"169.254.1.2 host.containers.internal host.docker.internal\n"
	if got != want {
		t.Errorf("renderContainerHosts empty = %q, want %q", got, want)
	}
}

func TestRenderContainerHosts_SitesPointAtNginx(t *testing.T) {
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "foo", Domains: []string{"foo.test"}},
		{Name: "bar", Domains: []string{"bar.test", "admin-bar.test"}},
	}}

	got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.2")

	// host.containers.internal must use the host gateway, never nginx.
	if !strings.Contains(got, "169.254.1.2 host.containers.internal host.docker.internal\n") {
		t.Errorf("missing host.containers.internal line:\n%s", got)
	}
	// .test domains must use the nginx container IP, never the host gateway.
	for _, d := range []string{"foo.test", "bar.test", "admin-bar.test"} {
		wantLine := "10.89.0.2 " + d + "\n"
		if !strings.Contains(got, wantLine) {
			t.Errorf("missing %q in output:\n%s", wantLine, got)
		}
		if strings.Contains(got, "169.254.1.2 "+d) {
			t.Errorf("site %q incorrectly points at host gateway:\n%s", d, got)
		}
	}
}

func TestRenderContainerHosts_DistinctIPs(t *testing.T) {
	// Regression: host-gateway (Xdebug) and nginx IP (.test) must stay separate.
	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "x", Domains: []string{"x.test"}},
	}}
	got := renderContainerHosts(reg, "169.254.1.2", "10.89.0.2")

	if strings.Contains(got, "169.254.1.2 x.test") {
		t.Errorf("x.test must not resolve to host gateway:\n%s", got)
	}
	if strings.Contains(got, "10.89.0.2 host.containers.internal") {
		t.Errorf("host.containers.internal must not resolve to nginx IP:\n%s", got)
	}
}

func TestRenderContainerHosts_PreservesLoopback(t *testing.T) {
	got := renderContainerHosts(&config.SiteRegistry{}, "1.2.3.4", "5.6.7.8")
	if !strings.HasPrefix(got, "127.0.0.1 localhost\n::1 localhost\n") {
		t.Errorf("loopback entries missing or out of order:\n%s", got)
	}
}

func TestHostCandidates(t *testing.T) {
	cases := []struct {
		name     string
		getentIP string
		lanIP    string
		want     []string
	}{
		{
			// Happy path: all three candidates present and distinct. Probe
			// order is preserved so host.containers.internal gets the first
			// shot — the address Xdebug docs already point users at, and the
			// canonical choice on macOS/gvproxy.
			name:     "all distinct",
			getentIP: "169.254.1.2",
			lanIP:    "192.168.1.10",
			want:     []string{"169.254.1.2", "192.168.1.10", "10.0.2.2"},
		},
		{
			// Regression: when getent and the LAN IP collide (some setups
			// alias them), the duplicate must be dropped so we don't probe
			// the same address twice and waste a 2 s timeout.
			name:     "getent and lan IP collide",
			getentIP: "192.168.1.10",
			lanIP:    "192.168.1.10",
			want:     []string{"192.168.1.10", "10.0.2.2"},
		},
		{
			// getent fails (no host.containers.internal entry, fresh install,
			// or container not yet up) — fall back to LAN IP plus slirp4netns
			// default. This is the path that rescues the rootless-netns case
			// behind issue #186 when netavark hasn't wired up an alias.
			name:     "getent missing",
			getentIP: "",
			lanIP:    "192.168.1.10",
			want:     []string{"192.168.1.10", "10.0.2.2"},
		},
		{
			// No LAN connection (offline laptop). Still try the addresses we
			// know about so an Xdebug-over-loopback setup keeps a chance.
			name:     "lan IP missing",
			getentIP: "169.254.1.2",
			lanIP:    "",
			want:     []string{"169.254.1.2", "10.0.2.2"},
		},
		{
			// Worst case: nothing detected. The slirp4netns default is the
			// only thing left to try. Better than returning an empty list,
			// because on slirp4netns systems 10.0.2.2 actually routes.
			name:     "nothing detected",
			getentIP: "",
			lanIP:    "",
			want:     []string{"10.0.2.2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hostCandidates(c.getentIP, c.lanIP)
			if len(got) != len(c.want) {
				t.Fatalf("hostCandidates(%q,%q) = %v, want %v", c.getentIP, c.lanIP, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("hostCandidates(%q,%q)[%d] = %q, want %q", c.getentIP, c.lanIP, i, got[i], c.want[i])
				}
			}
		})
	}
}

// setupHostsFixture points the config paths at a temp tree holding one linked
// site, and routes execCommand at a scripted podman. nginxIPs is consumed one
// entry per inspect of the nginx container's network, so a test can make
// consecutive inspects disagree the way a mid-write recreation would.
func setupHostsFixture(t *testing.T, nginxIPs ...string) *[]string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sites := "sites:\n  - name: blog\n    domains: [blog.test]\n    path: /home/u/blog\n"
	if err := os.MkdirAll(filepath.Dir(config.SitesFile()), 0755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(config.SitesFile(), []byte(sites), 0644); err != nil {
		t.Fatalf("write sites.yaml: %v", err)
	}

	var seen []string
	var inspectN int
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = func(name string, args ...string) *exec.Cmd {
		joined := strings.Join(args, " ")
		seen = append(seen, joined)
		switch {
		case strings.Contains(joined, "State.Running"):
			return fakeExec("true", "", 0)(name, args...)
		case strings.Contains(joined, "NetworkSettings.Networks"):
			ip := nginxIPs[len(nginxIPs)-1]
			if inspectN < len(nginxIPs) {
				ip = nginxIPs[inspectN]
			}
			inspectN++
			return fakeExec(ip, "", 0)(name, args...)
		case strings.Contains(joined, "getent"):
			return fakeExec("169.254.1.2 host.containers.internal", "", 0)(name, args...)
		case strings.Contains(joined, " nc "):
			return fakeExec("", "", 0)(name, args...)
		}
		return fakeExec("", "unexpected: "+joined, 1)(name, args...)
	}
	return &seen
}

func siteIPIn(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return parseNginxIP(data)
}

func countCalls(calls []string, needle string) int {
	n := 0
	for _, c := range calls {
		if strings.Contains(c, needle) {
			n++
		}
	}
	return n
}

// Both hosts files must be pinned to the same address. Resolving the IP once
// per file let a recreation land between the two lookups and leave PHP-FPM and
// Selenium pointed at different nginx containers.
func TestWriteContainerHosts_ResolvesNginxIPOncePerCall(t *testing.T) {
	calls := setupHostsFixture(t, "10.89.7.11", "10.89.7.99")

	if err := WriteContainerHosts(); err != nil {
		t.Fatalf("WriteContainerHosts: %v", err)
	}

	container := siteIPIn(t, config.ContainerHostsFile())
	browser := siteIPIn(t, config.BrowserHostsFile())
	if container != browser {
		t.Errorf("hosts files disagree on the nginx IP: container=%q browser=%q", container, browser)
	}
	if n := countCalls(*calls, "NetworkSettings.Networks"); n != 1 {
		t.Errorf("inspected the nginx IP %d times, want 1", n)
	}
}

// A caller repointing only the nginx IP must not disturb host.containers.internal.
// Re-detecting it inside the writer let the watcher's nginx path move the gateway
// without firing OnGatewayIPChange, stranding host-proxy vhosts on a dead address.
func TestWriteContainerHostsWith_PreservesTheGivenGateway(t *testing.T) {
	setupHostsFixture(t, "10.89.7.11")

	if err := WriteContainerHostsWith("192.168.1.50", "10.89.7.11"); err != nil {
		t.Fatalf("WriteContainerHostsWith: %v", err)
	}

	if got := ReadHostGatewayFromFile(); got != "192.168.1.50" {
		t.Errorf("gateway = %q, want the supplied %q", got, "192.168.1.50")
	}
	if got := siteIPIn(t, config.ContainerHostsFile()); got != "10.89.7.11" {
		t.Errorf("nginx IP = %q, want %q", got, "10.89.7.11")
	}
	if got := siteIPIn(t, config.BrowserHostsFile()); got != "10.89.7.11" {
		t.Errorf("browser-hosts nginx IP = %q, want %q", got, "10.89.7.11")
	}
}

// Writing with both addresses supplied must not shell out at all: the watcher
// calls this every time nginx moves, and the old path ran the reachability
// probe plus a `podman run --rm alpine` that can trigger an image pull.
func TestWriteContainerHostsWith_RunsNoPodman(t *testing.T) {
	calls := setupHostsFixture(t, "10.89.7.11")

	if err := WriteContainerHostsWith("169.254.1.2", "10.89.7.11"); err != nil {
		t.Fatalf("WriteContainerHostsWith: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("shelled out to podman %d times, want 0: %v", len(*calls), *calls)
	}
}

// The watcher calls this every tick from a background goroutine, so it needs
// the same wall-clock cap probeHostFromNginx gives the exec probe.
func TestLookupNginxContainerIP_CapsWallClock(t *testing.T) {
	prevExec, prevTimeout := execCommand, nginxInspectTimeout
	t.Cleanup(func() { execCommand, nginxInspectTimeout = prevExec, prevTimeout })
	nginxInspectTimeout = 50 * time.Millisecond
	execCommand = func(string, ...string) *exec.Cmd {
		return exec.Command("sleep", "30")
	}

	done := make(chan string, 1)
	go func() { done <- LookupNginxContainerIP() }()

	select {
	case got := <-done:
		if got != "" {
			t.Errorf("LookupNginxContainerIP() = %q, want %q on timeout", got, "")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no wall-clock cap; a hung podman blocks the watcher goroutine forever")
	}
}

// writeBrowserHosts relies on the loopback fallback to keep the file
// well-formed while nginx is down.
func TestNginxContainerIP_FallsBackToLoopback(t *testing.T) {
	prev := execCommand
	t.Cleanup(func() { execCommand = prev })
	execCommand = fakeExec("", "no such container", 1)

	if got := nginxContainerIP(); got != "127.0.0.1" {
		t.Errorf("nginxContainerIP() = %q, want %q when nginx is absent", got, "127.0.0.1")
	}
}

// ReadBrowserNginxIPFromFile reads the second bind-mounted file, which carries
// no gateway line of its own.
func TestReadBrowserNginxIPFromFile(t *testing.T) {
	setupHostsFixture(t, "10.89.7.11")

	if got := ReadBrowserNginxIPFromFile(); got != "" {
		t.Errorf("ReadBrowserNginxIPFromFile() with no file = %q, want %q", got, "")
	}
	if err := WriteContainerHostsWith("169.254.1.2", "10.89.7.11"); err != nil {
		t.Fatalf("WriteContainerHostsWith: %v", err)
	}
	if got := ReadBrowserNginxIPFromFile(); got != "10.89.7.11" {
		t.Errorf("ReadBrowserNginxIPFromFile() = %q, want %q", got, "10.89.7.11")
	}
}
