package goserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// servePaused answers every path of a paused site with its landing page, as
// the paused vhost's `try_files /paused.html =503` does.
func servePaused(w http.ResponseWriter) {
	page, err := os.ReadFile(filepath.Join(config.PausedDir(), "paused.html"))
	if err != nil {
		http.Error(w, "503 Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

// worktreeSite is the site as its git worktree on <branch>.<domain> serves
// it: the worktree's checkout and PHP version, the dev server port pinned for
// it. Only a subdomain host is looked up, so the main domain pays nothing.
func worktreeSite(site config.Site, domain, hostport string) (config.Site, bool) {
	host := strings.ToLower(hostOnly(hostport))
	if host == domain || !site.ServesWorkerSubdomains() {
		return site, false
	}
	wts, err := gitpkg.ServableWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return site, false
	}
	for _, wt := range wts {
		if wt.Domain != host {
			continue
		}
		wtSite := site
		wtSite.Path = wt.Path
		wtSite.PHPVersion = config.WorktreePHPVersion(wt.Path, site.PHPVersion)
		wtSite.DevServerPort = site.WorktreeDevPorts[filepath.Base(wt.Path)]
		return wtSite, true
	}
	return site, false
}
