package cli

import (
	"fmt"
	"io"

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
	moveEnsureImageFn = func() {
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
		rewriteUnmovedWorktrees(*s)
	}
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
