package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewRestartCmd returns the restart command.
func NewRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [site]",
		Short: "Restart the container for the current or named site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name, err := resolveSiteName(args)
			if err != nil {
				return err
			}
			feedback.Begin()
			return RestartSite(name)
		},
	}
}

// hostProxyStopTimeout bounds how long an explicit restart waits for the old
// process to exit and release its port. hostProxyRebindTimeout is the same wait
// on the gateway-rebind path, which runs unattended across every host-proxy site
// and cannot afford a long stall per site. hostProxyStopPoll is the gap between
// checks, and devServerPortInUse is the port probe, a seam for tests.
var (
	hostProxyStopTimeout   = 30 * time.Second
	hostProxyRebindTimeout = 2 * time.Second
	hostProxyStopPoll      = 250 * time.Millisecond
	devServerPortInUse     = func(port int) bool { return PortInUse(strconv.Itoa(port)) }
)

// restartDevServer stops a host-proxy site's dev server, gives the port a chance
// to come back before starting it again, and starts it either way. A process
// that outlives its unit leaves the port bound for a moment, and starting into
// that gets the new server killed on "address already in use". A port that never
// frees is usually something else holding it, which is not a reason to leave the
// site's only runtime stopped: the unit retries on its own until the port frees.
func restartDevServer(unit string, port int, wait time.Duration) error {
	if err := podman.StopUnit(unit); err != nil {
		return fmt.Errorf("stopping dev server: %w", err)
	}
	if !waitDevServerStopped(unit, port, wait) {
		feedback.Warn("port %d is still in use after %s, starting the dev server anyway; it retries until the port frees. Find what has it with: %s",
			port, wait, FindListenerCmd(strconv.Itoa(port)))
	}
	if err := podman.StartUnit(unit); err != nil {
		return fmt.Errorf("starting dev server: %w", err)
	}
	return nil
}

// waitDevServerStopped reports whether the unit went inactive and let go of its
// port within wait.
func waitDevServerStopped(unit string, port int, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		podman.InvalidateUnitStatusCache(unit)
		if !unitIsActiveOrActivating(unit) && (port <= 0 || !devServerPortInUse(port)) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(hostProxyStopPoll)
	}
}

// RestartSite restarts the custom container for a site. For PHP sites
// it restarts the shared FPM container for that site's PHP version.
func RestartSite(name string) error {
	site, err := config.FindSite(name)
	if err != nil {
		return fmt.Errorf("site %q not found", name)
	}

	if site.IsCustomContainer() {
		unit := podman.CustomContainerName(site.Name)
		if err := podman.RestartUnit(unit); err != nil {
			return fmt.Errorf("restarting container: %w", err)
		}
		feedback.Done("restarted " + feedback.Val(name) + " · " + unit)
		return nil
	}

	if site.IsFrankenPHP() {
		unit := podman.FrankenPHPContainerName(site.Name)
		if err := podman.RestartUnit(unit); err != nil {
			return fmt.Errorf("restarting FrankenPHP container: %w", err)
		}
		feedback.Done("restarted " + feedback.Val(name) + " · " + unit)
		return nil
	}

	if site.IsCustomFPM() {
		unit := podman.CustomFPMContainerName(site.Name)
		if err := podman.RestartUnit(unit); err != nil {
			return fmt.Errorf("restarting custom FPM container: %w", err)
		}
		feedback.Done("restarted " + feedback.Val(name) + " · " + unit)
		return nil
	}

	if site.IsHostProxy() {
		if site.HostCommand == "" {
			return fmt.Errorf("site %q is proxy-only (no command); lerd does not manage its process", name)
		}
		unit := hostProxyWorkerUnit(site.Name)
		if err := restartDevServer(unit, site.HostPort, hostProxyStopTimeout); err != nil {
			return err
		}
		feedback.Done("restarted " + feedback.Val(name) + " · " + unit)
		return nil
	}

	// A static site (no PHP, no container) is served directly by nginx and has
	// no per-site runtime. Restarting would bounce the shared FPM container,
	// disrupting every other PHP site on that version, so refuse instead.
	if !phpDet.SiteUsesPHP(*site) {
		return fmt.Errorf("site %q is static and has no container to restart", name)
	}
	if site.PHPVersion == "" {
		return fmt.Errorf("site %q has no PHP version set", name)
	}
	short := strings.ReplaceAll(site.PHPVersion, ".", "")
	unit := "lerd-php" + short + "-fpm"
	if err := podman.RestartUnit(unit); err != nil {
		return fmt.Errorf("restarting %s: %w", unit, err)
	}
	feedback.Done("restarted " + feedback.Val(name) + " · " + unit)
	return nil
}
