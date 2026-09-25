package siteops

import (
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/activityping"
	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/idle"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/reqstats"
)

// IsParkedSite checks whether a site's path is inside one of the parked directories.
func IsParkedSite(sitePath string, parkedDirs []string) bool {
	parent := filepath.Dir(sitePath)
	for _, dir := range parkedDirs {
		expanded := os.ExpandEnv(dir)
		if home, err := os.UserHomeDir(); err == nil {
			if len(expanded) > 0 && expanded[0] == '~' {
				expanded = filepath.Join(home, expanded[1:])
			}
		}
		if parent == expanded {
			return true
		}
	}
	return false
}

// StopSiteWorkers, when set, stops all running workers for a site as part of
// UnlinkSiteCore. It is wired up by the cli package (which owns worker
// lifecycle) at init time, mirroring podman.AfterUnitChange. Without it, the
// MCP and parked-watcher unlink paths — which call UnlinkSiteCore directly —
// would leave a host-proxy site's always-restart dev-server worker (and any
// framework workers) running after the site is gone.
var StopSiteWorkers func(site *config.Site)

// UnlinkSiteCore performs the shared unlink steps: stop workers, remove vhost,
// remove certs, update registry (ignore if parked, remove otherwise), update
// container hosts, and reload nginx.
func UnlinkSiteCore(site *config.Site, parkedDirs []string) error {
	TeardownSite(site, parkedDirs)

	if err := FinishSiteRemoval(); err != nil {
		return err
	}

	// See FinishLink: unlinking doesn't start/stop a systemd unit, so
	// the shared hook wouldn't otherwise fire. Notify explicitly.
	if podman.AfterUnitChange != nil {
		podman.AfterUnitChange("site:" + site.Name)
	}
	return nil
}

// TeardownSite applies everything an unlink does to one site and nothing that
// reaches past it. The install-wide tail is FinishSiteRemoval, so a sweep that
// reaps several deleted sites at once applies it once for all of them.
func TeardownSite(site *config.Site, parkedDirs []string) {
	if StopSiteWorkers != nil {
		StopSiteWorkers(site)
	}
	StopSiteShares(site.Name)

	_ = nginx.RemoveVhost(site.PrimaryDomain())
	// The site's worktrees are served by subdomain vhosts of their own, and
	// they go with it: a branch of a site lerd no longer knows about has
	// nothing left to serve.
	RemoveWorktreeVhosts(site.PrimaryDomain())

	if site.Secured {
		certs.RemoveSiteCerts(site.PrimaryDomain())
	}

	// Clean up the per-project custom container if this site uses one.
	// The image is kept so relinking is fast; use `lerd rebuild` to
	// force a fresh build.
	if site.IsCustomContainer() {
		_ = podman.StopUnit(podman.CustomContainerName(site.Name))
		podman.RemoveCustomContainer(site.Name)
		_ = podman.RemoveCustomContainerQuadlet(site.Name)
	}

	// Same cleanup for FrankenPHP sites: stop and remove the per-site
	// quadlet. The dunglas/frankenphp image is shared across all FrankenPHP
	// sites on this PHP version, so it stays in the local store.
	if site.IsFrankenPHP() {
		_ = podman.StopUnit(podman.FrankenPHPContainerName(site.Name))
		_ = podman.RemoveFrankenPHPQuadlet(site.Name)
	}

	// Custom-FPM PHP sites: stop the per-site FPM container and drop its
	// quadlet. The per-site image is kept so relinking is fast.
	if site.IsCustomFPM() {
		_ = podman.StopUnit(podman.CustomFPMContainerName(site.Name))
		_ = podman.RemoveCustomFPMQuadlet(site.Name)
	}

	if IsParkedSite(site.Path, parkedDirs) {
		_ = config.IgnoreSite(site.Name)
	} else {
		_ = config.RemoveSite(site.Name)
		_ = config.RemoveSiteFromWorkspaces(site.Name)
	}

	forgetSiteState(site.Name)
}

// FinishSiteRemoval applies what removing a site changes for the whole install:
// the container hosts files, the FPM quadlets, and the nginx reload that puts
// the dropped vhosts into effect.
func FinishSiteRemoval() error {
	_ = podman.WriteContainerHosts()
	_ = podman.RewriteFPMQuadlets()
	return nginx.ReloadAndSettle()
}

// forgetSiteState drops the per-site request-timing and idle state the watcher
// writes, which the rest of the unlink path leaves behind: the durable request
// store, both snapshot files, and (via the control socket) the running watcher's
// in-memory copy so it stops re-emitting the site. Worktree keys are covered too.
// All best-effort: a site with no recorded state or a down watcher just no-ops.
func forgetSiteState(name string) {
	_ = reqstats.RemoveSite(config.RequestStatsFile(), name)
	_ = idle.RemoveActivity(config.IdleActivityFile(), name)
	if _, err := os.Stat(config.RequestStatsDB()); err == nil {
		if st, err := reqstats.OpenShared(config.RequestStatsDB()); err == nil {
			_, _ = st.DeleteSite(name)
		}
	}
	activityping.Forget(name)
}
