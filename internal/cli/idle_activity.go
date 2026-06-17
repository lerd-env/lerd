package cli

import (
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/activityping"
	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// recordCwdActivity tells lerd-ui that the site containing dir is being worked
// on, so a CLI/shim command (php, artisan, composer, npm, tinker) keeps the site
// awake under idle-suspend the same way an HTTP request would — and wakes it if
// it was asleep. Best-effort: it pings the lerd-ui unix socket with a tight
// timeout and ignores every failure (lerd-ui not running, dir not in a site).
func recordCwdActivity(dir string) {
	if dir == "" {
		return
	}
	site, _ := config.FindSiteByPath(siteRootFor(dir))
	if site == nil {
		return
	}
	// A command run inside a git worktree checkout must wake that worktree's
	// own workers, which idle on an independent timer, not the parent site's.
	if key := worktreeActivityKey(site, dir); key != "" {
		activityping.Site(key)
		return
	}
	activityping.Site(site.Name)
}

// worktreeActivityKey returns the idle key (site/wtBase) for the site's worktree
// that contains dir, or "" when dir is in the main checkout or detection fails.
// The key format mirrors the one the idle engine derives so the activity ping
// lands on the worktree's record.
func worktreeActivityKey(site *config.Site, dir string) string {
	// recordCwdActivity fires on every php/node shim, so skip the worktree scan
	// for the common case where the command runs in the site's main checkout.
	if filepath.Clean(dir) == filepath.Clean(site.Path) {
		return ""
	}
	wts, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return ""
	}
	return matchWorktreeKey(site.Name, dir, wts)
}

// matchWorktreeKey returns site/wtBase when dir falls inside one of wts, else "".
func matchWorktreeKey(siteName, dir string, wts []gitpkg.Worktree) string {
	dir = filepath.Clean(dir)
	for _, wt := range wts {
		wtPath := filepath.Clean(wt.Path)
		if dir == wtPath || strings.HasPrefix(dir, wtPath+string(filepath.Separator)) {
			return siteName + "/" + config.WorktreeUnitSlug(filepath.Base(wt.Path))
		}
	}
	return ""
}
