package cli

import (
	"fmt"
	"io"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/webserver"
)

// Seams for moving nginx, so a test never rewrites or restarts the developer's own.
var (
	moveOnHostFn      = webserver.OnHost
	moveRewriteRestFn = rewriteUnmovedConfigs
	moveRewriteUnitFn = webserver.RewriteUnit
	moveRestartUnitFn = func() error {
		_ = podman.DaemonReloadFn()
		return podman.RestartUnit(podman.WebServerUnit)
	}
	moveEnsureImageFn = func() {
		if podman.LocalImageID(podman.ServiceImage(podman.WebServerUnit)) == "" {
			if err := podman.PullNginxImage(io.Discard); err != nil {
				feedback.Warn("pulling the nginx image: %v", err)
			}
		}
	}
)

// moveWebServer takes nginx along when a PHP runtime switch changes where it
// runs, reporting whether it did so the caller restarts nginx rather than
// reloads it. The moving sites' vhosts were rewritten already; the rest goes
// first, so the nginx that starts reads nothing written for its old home.
func moveWebServer(wasOnHost bool, sites []config.Site) bool {
	if moveOnHostFn() == wasOnHost {
		return false
	}
	if moveOnHostFn() {
		fmt.Println(" → moving nginx to the host, beside native PHP")
	} else {
		fmt.Println(" → moving nginx back to its container")
	}
	moveRewriteRestFn(sites)
	if !moveOnHostFn() {
		moveEnsureImageFn()
	}
	if _, err := moveRewriteUnitFn(); err != nil {
		feedback.Warn("writing the nginx unit: %v", err)
		return true
	}
	if err := moveRestartUnitFn(); err != nil {
		feedback.Warn("starting nginx: %v", err)
	}
	return true
}

// rewriteUnmovedConfigs writes the nginx configs a runtime switch does not:
// the server-wide ones, the dashboard and catch-all, service domains, and the
// vhosts of sites the switch skipped, paused ones and host proxies.
func rewriteUnmovedConfigs(sites []config.Site) {
	for _, step := range []func() error{webserver.EnsureConfig, webserver.EnsureDefaultSite, webserver.EnsureDashboard, serviceops.ApplyServiceDomains} {
		if err := step(); err != nil {
			feedback.Warn("rewriting nginx config: %v", err)
		}
	}
	for i := range sites {
		s := &sites[i]
		switch {
		case s.Ignored || switchableToNative(s):
			continue
		case s.Paused:
			if err := webserver.ApplyPausedSite(*s); err != nil {
				feedback.Warn("rewriting the paused page for %s: %v", s.Name, err)
			}
		default:
			if err := webserver.ApplySite(*s); err != nil {
				feedback.Warn("rewriting the vhost for %s: %v", s.Name, err)
			}
		}
	}
}
