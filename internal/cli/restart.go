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

// hostProxyStopTimeout bounds how long a dev-server restart waits for the old
// process to exit and release its port; hostProxyStopPoll is the gap between
// checks. devServerPortInUse is the port probe, a seam for tests.
var (
	hostProxyStopTimeout = 30 * time.Second
	hostProxyStopPoll    = 250 * time.Millisecond
	devServerPortInUse   = func(port int) bool { return PortInUse(strconv.Itoa(port)) }
)

// restartDevServer stops a host-proxy site's dev server, waits for it to really
// be gone, then starts it again. A straight unit restart re-execs the command
// while the old process is still draining its queues, and the new one dies on
// "address already in use", so the port has to come back before the start.
func restartDevServer(unit string, port int) error {
	if err := podman.StopUnit(unit); err != nil {
		return fmt.Errorf("stopping dev server: %w", err)
	}
	if !waitDevServerStopped(unit, port) {
		return fmt.Errorf("dev server still holds port %d after %s; find what has it with: %s",
			port, hostProxyStopTimeout, FindListenerCmd(strconv.Itoa(port)))
	}
	if err := podman.StartUnit(unit); err != nil {
		return fmt.Errorf("starting dev server: %w", err)
	}
	return nil
}

// waitDevServerStopped reports whether the unit went inactive and let go of its
// port within hostProxyStopTimeout.
func waitDevServerStopped(unit string, port int) bool {
	deadline := time.Now().Add(hostProxyStopTimeout)
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
		if err := restartDevServer(unit, site.HostPort); err != nil {
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
