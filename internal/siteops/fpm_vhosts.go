package siteops

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nginx"
)

// ServedByFPM reports whether a site's requests go through an FPM vhost, the
// one place the SPX cookie and the browser logs script are injected.
func ServedByFPM(s config.Site) bool {
	return !s.IsCustomContainer() && !s.IsFrankenPHP() && !s.IsHostProxy()
}

// RegenerateFPMVhosts rewrites the vhost of every active PHP-FPM site and its
// worktrees, so a global toggle the templates read (the profiler, browser
// capture) takes effect. Paused, ignored, custom-container and FrankenPHP
// sites are skipped: they have no FPM vhost, and regenerating a paused site
// would revive it.
func RegenerateFPMVhosts() error {
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}
	for i := range reg.Sites {
		s := reg.Sites[i]
		if s.Ignored || s.Paused || !ServedByFPM(s) {
			continue
		}
		if err := RegenerateFPMSiteVhosts(s); err != nil {
			return err
		}
	}
	return nil
}

// RegenerateFPMSiteVhosts rewrites one FPM site's vhost and its worktrees'.
func RegenerateFPMSiteVhosts(s config.Site) error {
	if err := RegenerateSiteVhost(&s, s.PrimaryDomain()); err != nil {
		return fmt.Errorf("regenerating vhost for %s: %w", s.Name, err)
	}
	// Worktree vhosts share the site template, so they need the toggle too.
	worktrees, err := gitpkg.DetectWorktrees(s.Path, s.PrimaryDomain())
	if err != nil {
		return nil
	}
	for _, wt := range worktrees {
		php := config.WorktreePHPVersion(wt.Path, s.PHPVersion)
		_ = nginx.GenerateWorktreeVhostFor(wt.Domain, wt.Path, php, s.PrimaryDomain(), s.Name, wt.Branch, s.Secured)
	}
	return nil
}
