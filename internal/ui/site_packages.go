package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/geodro/lerd/internal/config"
)

// packagesRoute serves the composer packages a site's framework suggests:
//
//	GET  /api/sites/{d}/packages                  the ones still to offer
//	POST /api/sites/{d}/packages/install?name=…   composer require, streamed
//	POST /api/sites/{d}/packages/dismiss?name=…   stop offering it here
func packagesRoute(w http.ResponseWriter, r *http.Request, domain string, rest []string) bool {
	if len(rest) == 0 || rest[0] != "packages" {
		return false
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, map[string]any{"error": "site not found: " + domain})
		return true
	}
	switch {
	case len(rest) == 1 && r.Method == http.MethodGet:
		writeJSON(w, map[string]any{"packages": pendingPackages(site)})
	case len(rest) == 2 && r.Method == http.MethodPost && (rest[1] == "install" || rest[1] == "dismiss"):
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return true
		}
		// Only a package the site is being offered can be installed or
		// dismissed, so the name never comes from the client alone.
		name := r.URL.Query().Get("name")
		i := slices.IndexFunc(pendingPackages(site), func(p config.PackageSuggestion) bool { return p.Name == name })
		if i < 0 {
			writeJSON(w, map[string]any{"error": "not a package suggested for this site: " + name})
			return true
		}
		if rest[1] == "dismiss" {
			if err := config.DismissSitePackage(site.Name, name); err != nil {
				writeJSON(w, map[string]any{"error": err.Error()})
				return true
			}
			writeJSON(w, map[string]any{"ok": true})
			return true
		}
		release, busyWith, ok := tryAcquireRun(siteRunLockKey(site), "install "+name)
		if !ok {
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, map[string]any{"error": "another command is already running on this site: " + busyWith})
			return true
		}
		defer release()
		streamShellRun(w, r.Context(), site.Path, pendingPackages(site)[i].InstallCommand(), false)
	default:
		http.NotFound(w, r)
	}
	return true
}

// pendingPackages lists the packages the site's framework suggests that its
// composer project does not have installed and the user has not turned down.
func pendingPackages(site *config.Site) []config.PackageSuggestion {
	out := []config.PackageSuggestion{}
	if _, err := os.Stat(filepath.Join(site.Path, "composer.json")); err != nil {
		return out
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok || fw == nil {
		return out
	}
	for _, p := range fw.SuggestPackages {
		if p.Valid() && !slices.Contains(site.DismissedPackages, p.Name) && !config.ComposerHasInstalled(site.Path, p.Name, "require-dev") {
			out = append(out, p)
		}
	}
	return out
}
