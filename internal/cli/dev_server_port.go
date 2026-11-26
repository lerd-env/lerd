package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/freeport"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteops"
)

// Seams for tests. devServerPortWait is how long a stopped server's own process
// gets to let go of its port before the port counts as taken.
var (
	devServerPortFree = freeport.Bindable
	devServerPortWait = 5 * time.Second
	devServerRunning  = func(unit string) bool {
		podman.InvalidateUnitStatusCache(unit)
		st, _ := podman.UnitStatus(unit)
		return st == "active"
	}
	devServerRepoint = func(site *config.Site) error {
		// A waking or paused page stays up: the resume that ends it writes the vhost.
		conf, err := os.ReadFile(filepath.Join(config.NginxConfD(), site.PrimaryDomain()+".conf"))
		if err == nil && !hostProxyPassRe.Match(conf) {
			return nil
		}
		if err := siteops.RegenerateSiteVhost(site, site.PrimaryDomain()); err != nil {
			return err
		}
		if up, _ := podman.ContainerRunning("lerd-nginx"); up {
			return nginx.Reload()
		}
		return nil
	}
)

// hostProxyPassRe matches a vhost that proxies to the host, on Linux by address
// and on macOS by name.
var hostProxyPassRe = regexp.MustCompile(`proxy_pass\s+https?://([0-9.]+|host\.containers\.internal):\d+;`)

// liveProxy is the site's proxy block on the port its dev server really has.
func liveProxy(site config.Site, proxy *config.ProxyConfig) *config.ProxyConfig {
	if site.HostPortMovedFrom == 0 || site.HostPortMovedFrom != proxy.Port {
		return proxy
	}
	moved := *proxy
	moved.Port = site.HostPort
	return &moved
}

// settleDevServerPort picks the port a stopped dev server is about to start on:
// the declared one when it is free, another when the framework allows moving.
// The registry and the vhost follow. Reports whether the port changed.
func settleDevServerPort(site *config.Site, proxy *config.ProxyConfig) bool {
	// A site linked another way stays that way: only a link makes it a proxy.
	if proxy.Command == "" || !site.IsHostProxy() {
		return false
	}
	unlock, err := config.LockDevServerPorts()
	if err != nil {
		feedback.Warn("%v", err)
		return false
	}
	defer unlock()
	// Another process may have moved it since the caller read the site.
	callerPort := site.HostPort
	if fresh, err := config.FindSite(site.Name); err == nil {
		site.HostPort, site.HostPortMovedFrom = fresh.HostPort, fresh.HostPortMovedFrom
	}
	if devServerRunning(hostProxyWorkerUnit(site.Name)) {
		return false
	}
	port := devServerPort(*site, proxy)
	movedFrom := 0
	if port != proxy.Port {
		movedFrom = proxy.Port
	}
	if port == site.HostPort && movedFrom == site.HostPortMovedFrom {
		if port != callerPort {
			repointDevServer(site)
		}
		return port != callerPort
	}
	if err := config.SetSiteHostPort(site.Name, port, movedFrom); err != nil {
		feedback.Warn("recording the dev server port of %s: %v", site.Name, err)
		return false
	}
	site.HostPort, site.HostPortMovedFrom = port, movedFrom
	repointDevServer(site)
	if movedFrom > 0 {
		feedback.Warn("port %d is taken, so %s runs on port %d until it is free", movedFrom, site.Name, port)
	}
	return true
}

func repointDevServer(site *config.Site) {
	if err := devServerRepoint(site); err != nil {
		feedback.Warn("pointing nginx at port %d for %s: %v", site.HostPort, site.Name, err)
	}
}

func devServerPort(site config.Site, proxy *config.ProxyConfig) int {
	movable := devServerMovable(site, proxy)
	held, _ := siteHostPorts(site.Name)
	available := func(port int) bool {
		return devServerPortFree(port) && !(movable && held[port])
	}
	moved := site.HostPortMovedFrom == proxy.Port && site.HostPort != proxy.Port
	if available(proxy.Port) || (movable && !moved && waitForPort(proxy.Port, available)) {
		return proxy.Port
	}
	if !movable {
		feedback.Warn("port %d of %s is taken by another process. Find it with: %s",
			proxy.Port, site.Name, FindListenerCmd(strconv.Itoa(proxy.Port)))
		return proxy.Port
	}
	if moved && available(site.HostPort) {
		return site.HostPort
	}
	// No site is excepted: this one's own worktrees hold ports too.
	return allocateHostPort(proxy.Port+1, "")
}

func waitForPort(port int, available func(int) bool) bool {
	for deadline := time.Now().Add(devServerPortWait); time.Now().Before(deadline); {
		time.Sleep(hostProxyStopPoll)
		if available(port) {
			return true
		}
	}
	return false
}

func devServerMovable(site config.Site, proxy *config.ProxyConfig) bool {
	fw, _ := config.GetFrameworkForDir(site.Framework, site.Path)
	return proxy.CanMovePort(fw)
}

// restartDevServerMoving restarts a movable dev server through the start path,
// which settles its port. False leaves the plain restart to the caller.
func restartDevServerMoving(site *config.Site, unit string) (bool, error) {
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil || proj == nil || proj.Proxy == nil || proj.Proxy.Command != site.HostCommand {
		return false, nil
	}
	if !devServerMovable(*site, proj.Proxy) {
		return false, nil
	}
	// Asked before the stop, so a refused command leaves the server running.
	if err := gateHostProxyAutostart(*site, proj.Proxy.Command); err != nil {
		return false, err
	}
	if err := podman.StopUnit(unit); err != nil {
		return false, fmt.Errorf("stopping dev server: %w", err)
	}
	if err := launchHostProxy(site, proj.Proxy); err != nil {
		return false, fmt.Errorf("starting dev server: %w", err)
	}
	return true, nil
}
