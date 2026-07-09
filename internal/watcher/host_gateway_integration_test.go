package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

const (
	staleIP = "10.89.7.143"
	freshIP = "10.89.7.11"
	gwLine  = "169.254.1.2 host.containers.internal host.docker.internal\n"
)

// hostsEnv is a temp XDG tree with one linked site, both hosts files pinned to
// staleIP, and a fake podman on PATH whose reported nginx IP the test controls.
type hostsEnv struct {
	dir     string
	ipFile  string
	callLog string
}

// newHostsEnv installs the fake podman and seeds the files the tick reads. The
// fake answers only `inspect`, which is the sole podman call the nginx path
// makes once WriteContainerHostsWith stops re-resolving addresses.
func newHostsEnv(t *testing.T, nginxIP string) *hostsEnv {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	env := &hostsEnv{
		dir:     dir,
		ipFile:  filepath.Join(dir, "nginx-ip"),
		callLog: filepath.Join(dir, "podman-calls.log"),
	}
	env.setNginxIP(t, nginxIP)

	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	// Shell builtins only (no cat/printf lookup), so the script does not depend
	// on PATH. Exits non-zero when the IP file is empty, standing in for a
	// stopped or absent container. Records argv so a test can count calls.
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + env.callLog + "\n" +
		"read ip < " + env.ipFile + " || true\n" +
		"[ -z \"$ip\" ] && exit 1\n" +
		"echo \"$ip\"\n"
	if err := os.WriteFile(filepath.Join(bin, "podman"), []byte(script), 0755); err != nil {
		t.Fatalf("write fake podman: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	sites := "sites:\n  - name: blog\n    domains: [blog.test]\n    path: /home/u/blog\n"
	writeFile(t, config.SitesFile(), sites, 0644)
	writeFile(t, config.ContainerHostsFile(),
		"127.0.0.1 localhost\n::1 localhost\n"+gwLine+staleIP+" blog.test\n", 0644)
	writeFile(t, config.BrowserHostsFile(),
		"127.0.0.1 localhost\n::1 localhost\n"+staleIP+" blog.test\n", 0644)
	return env
}

func (e *hostsEnv) setNginxIP(t *testing.T, ip string) {
	t.Helper()
	writeFile(t, e.ipFile, ip, 0644)
}

func (e *hostsEnv) podmanCalls(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(e.callLog)
	if err != nil {
		return 0
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// realTick runs one tick through the production podman-backed dependencies,
// with the gateway half parked on its fast path.
func realTick(t *testing.T) {
	t.Helper()
	lan := primaryLANIP()
	tickHostGateway(hostGatewayDepsFromPodman(), &hostGatewayState{lastLAN: lan})
}

// A moved nginx address repoints both bind-mounted files, leaves the gateway
// line untouched so host-proxy vhosts stay valid, and costs one inspect.
func TestIntegration_TickRepointsBothFilesAndKeepsGateway(t *testing.T) {
	env := newHostsEnv(t, freshIP)

	realTick(t)

	container := readFile(t, config.ContainerHostsFile())
	browser := readFile(t, config.BrowserHostsFile())
	for name, got := range map[string]string{"hosts": container, "browser-hosts": browser} {
		if !strings.Contains(got, freshIP+" blog.test") {
			t.Errorf("%s not repointed at %s:\n%s", name, freshIP, got)
		}
		if strings.Contains(got, staleIP) {
			t.Errorf("%s still carries the stale %s:\n%s", name, staleIP, got)
		}
	}
	if !strings.Contains(container, gwLine) {
		t.Errorf("gateway line was rewritten; want it byte-identical:\n%s", container)
	}
	if n := env.podmanCalls(t); n != 1 {
		t.Errorf("podman ran %d times, want exactly 1 (the inspect)", n)
	}
}

// A tick that lands while nginx is down must leave both files alone rather
// than baking the 127.0.0.1 fallback into every container.
func TestIntegration_NginxDownLeavesFilesUntouched(t *testing.T) {
	env := newHostsEnv(t, freshIP)
	env.setNginxIP(t, "") // fake podman now exits 1

	before := readFile(t, config.ContainerHostsFile())
	realTick(t)

	if got := readFile(t, config.ContainerHostsFile()); got != before {
		t.Errorf("hosts file changed while nginx was down:\n%s", got)
	}
	if got := readFile(t, config.BrowserHostsFile()); strings.Contains(got, "127.0.0.1 blog.test") {
		t.Errorf("baked loopback into browser-hosts:\n%s", got)
	}
}

// browser-hosts can go missing on its own. The staleness check reads both
// files, so the next tick regenerates it even though hosts is already correct.
func TestIntegration_RegeneratesMissingBrowserHosts(t *testing.T) {
	newHostsEnv(t, freshIP)
	writeFile(t, config.ContainerHostsFile(),
		"127.0.0.1 localhost\n::1 localhost\n"+gwLine+freshIP+" blog.test\n", 0644)
	if err := os.Remove(config.BrowserHostsFile()); err != nil {
		t.Fatalf("remove browser-hosts: %v", err)
	}

	realTick(t)

	if got := readFile(t, config.BrowserHostsFile()); !strings.Contains(got, freshIP+" blog.test") {
		t.Errorf("browser-hosts not regenerated:\n%s", got)
	}
}

// WriteContainerHostsWith writes hosts before browser-hosts. When the second
// write fails the first already carries the fresh IP; the next tick must still
// retry rather than mistake the pair for fresh.
func TestIntegration_RetriesAfterPartialWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-only mode bit, so the write never fails")
	}
	newHostsEnv(t, freshIP)
	if err := os.Chmod(config.BrowserHostsFile(), 0444); err != nil {
		t.Fatalf("chmod browser-hosts: %v", err)
	}

	realTick(t) // hosts lands, browser-hosts is refused

	if got := readFile(t, config.ContainerHostsFile()); !strings.Contains(got, freshIP) {
		t.Fatalf("hosts should have landed on the first tick:\n%s", got)
	}
	if got := readFile(t, config.BrowserHostsFile()); !strings.Contains(got, staleIP) {
		t.Fatalf("browser-hosts should still be stale:\n%s", got)
	}

	if err := os.Chmod(config.BrowserHostsFile(), 0644); err != nil {
		t.Fatalf("chmod browser-hosts: %v", err)
	}
	realTick(t)

	if got := readFile(t, config.BrowserHostsFile()); !strings.Contains(got, freshIP+" blog.test") {
		t.Errorf("second tick did not retry the failed browser-hosts write:\n%s", got)
	}
}

// With no sites linked there is no pinned address to compare against, so the
// tick must not rewrite anything.
func TestIntegration_NoSitesNoWrite(t *testing.T) {
	env := newHostsEnv(t, freshIP)
	writeFile(t, config.SitesFile(), "sites: []\n", 0644)
	writeFile(t, config.ContainerHostsFile(), "127.0.0.1 localhost\n::1 localhost\n"+gwLine, 0644)
	before := readFile(t, config.ContainerHostsFile())

	realTick(t)

	if got := readFile(t, config.ContainerHostsFile()); got != before {
		t.Errorf("rewrote the hosts file with no sites linked:\n%s", got)
	}
	if n := env.podmanCalls(t); n != 1 {
		t.Errorf("podman ran %d times, want exactly 1", n)
	}
}

// The loop itself, not a hand-called tick: WatchHostGateway must notice the
// address moved and repoint without anyone driving it.
func TestIntegration_WatchLoopRepointsOnItsOwn(t *testing.T) {
	env := newHostsEnv(t, staleIP)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		WatchHostGateway(10*time.Millisecond, stop)
	}()

	env.setNginxIP(t, freshIP)

	deadline := time.After(5 * time.Second)
	for {
		if strings.Contains(readFile(t, config.ContainerHostsFile()), freshIP+" blog.test") &&
			strings.Contains(readFile(t, config.BrowserHostsFile()), freshIP+" blog.test") {
			break
		}
		select {
		case <-deadline:
			close(stop)
			<-done
			t.Fatal("watch loop never repointed the hosts files")
		case <-time.After(10 * time.Millisecond):
		}
	}

	close(stop)
	<-done

	if got := readFile(t, config.ContainerHostsFile()); !strings.Contains(got, gwLine) {
		t.Errorf("watch loop rewrote the gateway line:\n%s", got)
	}
}
