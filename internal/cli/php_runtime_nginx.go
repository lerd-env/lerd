package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
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
	moveContainerHostsFn = podman.WriteContainerHosts
	moveEnsureImageFn    = func() {
		if podman.LocalImageID(podman.ServiceImage(podman.WebServerUnit)) == "" {
			if err := podman.PullNginxImage(io.Discard); err != nil {
				feedback.Warn("pulling the nginx image: %v", err)
			}
		}
	}
)

// moveWebServer takes nginx along when a runtime switch changes where it runs,
// reporting it so the caller restarts rather than reloads. Everything else is
// rewritten first, so the nginx that starts reads nothing meant for its old home.
func moveWebServer(moved bool, sites []config.Site) bool {
	if !moved {
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
	// After the restart, so the address containers get for .test is the new home's.
	if err := moveContainerHostsFn(); err != nil {
		feedback.Warn("pointing containers at nginx: %v", err)
	}
	return true
}

// rewriteUnmovedConfigs writes the nginx configs a runtime switch does not:
// the server-wide ones, the dashboard and catch-all, service domains, and the
// vhosts of sites the switch skipped, paused ones and host proxies.
func rewriteUnmovedConfigs(sites []config.Site) {
	for _, step := range []func() error{webserver.EnsureConfig, webserver.EnsureDefaultSite, webserver.EnsureDashboard, webserver.RewriteNativeUpstreams, serviceops.ApplyServiceDomains} {
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
			if s.IsFrankenPHP() && moveOnHostFn() {
				publishFrankenPHP(*s)
			}
			if err := webserver.ApplySite(*s); err != nil {
				feedback.Warn("rewriting the vhost for %s: %v", s.Name, err)
			}
			// The switch's own sweep reaches only the sites it moves to native PHP.
			if s.IsFrankenPHP() {
				regenerateWorktreeVhosts(s)
			}
		}
		rewriteUnmovedWorktrees(*s)
	}
}

// refreshFrankenPHPFn rewrites a FrankenPHP quadlet, a seam so tests restart nothing.
// It restarts on what the running container publishes, not on whether this write
// changed the file: an earlier writer may have changed it without a restart.
var refreshFrankenPHPFn = func(s config.Site) error {
	changed, err := podman.RefreshFrankenPHPQuadlet(s)
	if err != nil {
		return err
	}
	port, err := podman.FrankenPHPHostPort(s.Name)
	if err != nil {
		return err
	}
	unit := podman.FrankenPHPContainerName(s.Name)
	if !changed && frankenPHPPublishes(unit, port) {
		return nil
	}
	_ = podman.DaemonReloadFn()
	return podman.RestartUnit(unit)
}

// frankenPHPPublishes reports whether the container is running and serves its
// port on the loopback one the host's nginx proxies to.
func frankenPHPPublishes(unit string, hostPort int) bool {
	if running, _ := podman.ContainerRunning(unit); !running {
		return false
	}
	out, err := podman.Run("port", unit, strconv.Itoa(podman.FrankenPHPPort))
	return err == nil && strings.Contains(out, fmt.Sprintf("127.0.0.1:%d", hostPort))
}

// publishFrankenPHP restarts a FrankenPHP container on the loopback port the
// host's nginx proxies to, when its quadlet did not publish one yet. It
// reports whether the container is up on it, started here or already.
func publishFrankenPHP(s config.Site) bool {
	if err := refreshFrankenPHPFn(s); err != nil {
		feedback.Warn("publishing %s for the host's nginx: %v", s.Name, err)
		return false
	}
	return true
}

var (
	unmovedWorktreesFn         = gitpkg.DetectWorktrees
	unmovedPausedWorktreeFn    = webserver.ApplyPausedWorktree
	unmovedHostProxyWorktreeFn = func(s config.Site, path, domain string, secured bool) error {
		return certs.RegenerateHostProxyWorktreeVhost(s, path, domain, secured)
	}
)

// rewriteUnmovedWorktrees rewrites the worktrees of a site the switch skipped
// in the site's own form, paused or proxied.
func rewriteUnmovedWorktrees(s config.Site) {
	if !s.Paused && !s.IsHostProxy() {
		return
	}
	wts, err := unmovedWorktreesFn(s.Path, s.PrimaryDomain())
	if err != nil {
		return
	}
	for _, wt := range wts {
		if s.Paused {
			err = unmovedPausedWorktreeFn(wt.Domain, s.PrimaryDomain(), config.PausedDir(), s.Secured)
		} else {
			err = unmovedHostProxyWorktreeFn(s, wt.Path, wt.Domain, s.Secured)
		}
		if err != nil {
			feedback.Warn("rewriting the vhost for worktree %s: %v", wt.Domain, err)
		}
	}
}

// followNginxMove rewrites what install's own site loop leaves behind when it
// moves nginx: the active sites' worktrees, then everything the switch skips.
func followNginxMove(sites []config.Site) {
	for i := range sites {
		if switchableToNative(&sites[i]) {
			regenerateWorktreeVhosts(&sites[i])
		}
	}
	moveRewriteRestFn(sites)
}
