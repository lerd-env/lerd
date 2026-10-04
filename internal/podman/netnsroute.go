package podman

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/platform"
)

// ErrNetnsHeld means the rootless network namespace survived stopping every
// lerd container, so something outside lerd still has a container on a bridge
// network and the namespace cannot be rebuilt.
var ErrNetnsHeld = errors.New("a container outside lerd keeps podman's rootless network alive, stop it and run lerd start again")

// Seams over the host probes, so the check and heal can be driven by tests.
var (
	rootlessNetnsUpFn     = rootlessNetnsUp
	hostHasDefaultRouteFn = hostHasDefaultRoute
	netnsDefaultRouteFn   = netnsDefaultRoute
	runningOnNetworkFn    = runningOnNetwork
	stopUnitFn            = StopUnit
	startUnitFn           = StartUnit
)

// RootlessNetnsLacksDefaultRoute reports whether pasta built the shared
// rootless netns before the host had a default route (#2047). pasta templates
// the namespace on whatever interface it finds at that moment, a link-down
// docker0 at boot for example, and never follows the host afterwards, so every
// container loses the internet while the host itself is online.
func RootlessNetnsLacksDefaultRoute() bool {
	if !rootlessNetnsUpFn() || !hostHasDefaultRouteFn() {
		return false
	}
	routes, err := netnsDefaultRouteFn()
	return err == nil && strings.TrimSpace(routes) == ""
}

// HealRoutelessNetns rebuilds a rootless netns that has no default route. The
// namespace lives only as long as a bridge container runs, so stopping every
// lerd container on the lerd network lets pasta exit, and the first start
// brings up a fresh one templated on the host's current interface.
func HealRoutelessNetns() (bool, error) {
	if !RootlessNetnsLacksDefaultRoute() {
		return false, nil
	}
	attached, err := runningOnNetworkFn("lerd")
	if err != nil {
		return false, err
	}
	for _, c := range attached {
		_ = stopUnitFn(c)
	}
	// Probing again joins the old namespace if something still holds it, or
	// builds a fresh one on the host's current route if nothing does.
	held := RootlessNetnsLacksDefaultRoute()
	for _, c := range attached {
		if err := startUnitFn(c); err != nil {
			return false, fmt.Errorf("restarting %s: %w", c, err)
		}
	}
	if held {
		return false, ErrNetnsHeld
	}
	return true, nil
}

// rootlessNetnsUp is false off Linux, where podman runs inside a VM and the
// namespace is not the host's to inspect.
func rootlessNetnsUp() bool {
	dir := rootlessNetnsDir()
	if platform.Current.UsesMachineVM || dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "rootless-netns-conn.pid"))
	return err == nil
}

func hostHasDefaultRoute() bool {
	data, _ := os.ReadFile("/proc/net/route")
	return procRouteHasDefault(string(data))
}

// procRouteHasDefault reads the kernel's IPv4 route table as /proc/net/route
// prints it; a default route is the one with destination 00000000.
func procRouteHasDefault(table string) bool {
	for _, line := range strings.Split(table, "\n")[1:] {
		if f := strings.Fields(line); len(f) > 1 && f[1] == "00000000" {
			return true
		}
	}
	return false
}

func netnsDefaultRoute() (string, error) {
	return Run("unshare", "--rootless-netns", "ip", "-4", "route", "show", "default")
}

func runningOnNetwork(name string) ([]string, error) {
	out, err := Run("ps", "--filter", "network="+name, "--format", "{{.Names}}")
	if err != nil {
		return nil, fmt.Errorf("listing containers on %s: %w", name, err)
	}
	return strings.Fields(out), nil
}
