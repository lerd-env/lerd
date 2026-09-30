package cli

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteops"
)

// Wire the dev server refresh to the siteops paths that move a site's addresses,
// securing it and changing its domains, which the CLI, the UI and MCP all share.
// The mechanism lives in this package, so the hook is filled in from here.
func init() {
	siteops.RefreshDevServers = RefreshDevServers
	siteops.StopSiteShares = stopSiteShares
	siteops.ResyncSiteWorkers = resyncSiteWorkers
}

// stopSiteShares releases every listener a site holds, its worktrees' included,
// so unlinking it cannot leave one bound with no registry entry left to reach it
// by. Worktree shares are keyed per branch, so they need releasing by name.
func stopSiteShares(siteName string) {
	LANShareStopServer(siteName)
	LANShareStopWorktrees(siteName)
	PublicShareStopServer(siteName)
	StopSiteTunnels(siteName)
}

// resyncSiteWorkers rewrites every running worker of a site for the PHP version
// the site now records, restarting only the units whose ExecStart changed.
// Idle-suspended workers stay down, as they do on the host-worker resync.
func resyncSiteWorkers(s *config.Site) {
	if s.Paused || s.Ignored {
		return
	}
	fw, ok := config.GetFrameworkForDir(s.Framework, s.Path)
	if !ok || fw.Workers == nil {
		return
	}
	for w, wDef := range fw.Workers {
		if containsString(s.IdleSuspendedWorkers, w) {
			continue
		}
		regenerateWorkerUnit(s.Name, s.Path, s.PHPVersion, w, wDef, "lerd-"+w+"-"+s.Name)
	}
}
