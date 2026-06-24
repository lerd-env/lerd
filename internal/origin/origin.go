// Package origin centralises every URL lerd fetches its own assets from (release
// binaries, framework store, changelog, GHCR base images) and owns the
// geodro->lerd-env move: old org first, new as fallback, flipping on a new-org hit.
package origin

import (
	"os"
	"strings"
	"sync"
)

// org holds the repo coordinates for one GitHub owner.
type org struct {
	owner          string // GitHub org, also the GHCR namespace
	mainRepo       string // owner/name for releases, installer, changelog
	frameworksRepo string // owner/name for the framework store
}

// Links is the centralized handler for lerd's distribution URLs and the
// old->new org switch. The zero value is not usable; use New or Default.
type Links struct {
	old, new  org
	mu        sync.RWMutex
	preferNew bool // when true, the new org is served first
}

// New builds a Links with the geodro (old) and lerd-env (new) coordinates,
// preferring old first unless LERD_DISTRIBUTION_ORG says otherwise.
func New() *Links {
	l := &Links{
		// geodro is the current (old) org, served first and kept as the fallback.
		old: org{owner: "geodro", mainRepo: "geodro/lerd", frameworksRepo: "geodro/lerd-frameworks"},
		new: org{owner: "lerd-env", mainRepo: "lerd-env/lerd", frameworksRepo: "lerd-env/frameworks"},
	}
	switch strings.ToLower(os.Getenv("LERD_DISTRIBUTION_ORG")) {
	case "lerd-env", "new":
		l.preferNew = true
	case "geodro", "old":
		l.preferNew = false
	}
	return l
}

// Default is the process-wide Links used by the package-level helpers.
var Default = New()

// PreferNew reports whether the new (lerd-env) org is served first.
func (l *Links) PreferNew() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.preferNew
}

// SetPreferNew sets which org is served first.
func (l *Links) SetPreferNew(v bool) {
	l.mu.Lock()
	l.preferNew = v
	l.mu.Unlock()
}

// ordered returns [preferred, other] for a function that maps an org to a URL.
func (l *Links) ordered(of func(org) string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.preferNew {
		return []string{of(l.new), of(l.old)}
	}
	return []string{of(l.old), of(l.new)}
}

func rawRepo(o org) string  { return "https://raw.githubusercontent.com/" + o.frameworksRepo }
func rawMain(o org) string  { return "https://raw.githubusercontent.com/" + o.mainRepo }
func releases(o org) string { return "https://github.com/" + o.mainRepo + "/releases" }
func apiBase(o org) string  { return "https://api.github.com/repos/" + o.mainRepo }

// StoreBaseURLs lists framework-store bases in priority order.
func (l *Links) StoreBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_STORE_BASE_URL")); len(list) > 0 {
		return list
	}
	return l.ordered(func(o org) string { return rawRepo(o) + "/main/frameworks" })
}

// ReleaseBaseURLs lists GitHub releases bases in priority order.
func (l *Links) ReleaseBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_RELEASES_URL")); len(list) > 0 {
		return list
	}
	return l.ordered(releases)
}

// ReleaseDownloadBases lists release-asset download bases in priority order.
func (l *Links) ReleaseDownloadBases() []string {
	if list := splitList(os.Getenv("LERD_RELEASE_DOWNLOAD_URL")); len(list) > 0 {
		return list
	}
	out := l.ReleaseBaseURLs()
	for i := range out {
		out[i] += "/download"
	}
	return out
}

// ReleaseAPIBaseURLs lists GitHub API bases in priority order.
func (l *Links) ReleaseAPIBaseURLs() []string {
	if list := splitList(os.Getenv("LERD_RELEASES_API_URL")); len(list) > 0 {
		return list
	}
	return l.ordered(apiBase)
}

// ChangelogURLs lists raw changelog URLs in priority order.
func (l *Links) ChangelogURLs() []string {
	if list := splitList(os.Getenv("LERD_CHANGELOG_URL")); len(list) > 0 {
		return list
	}
	return l.ordered(func(o org) string { return rawMain(o) + "/main/CHANGELOG.md" })
}

// BaseImageRefs lists GHCR refs for a prebuilt PHP-FPM base image in priority
// order, where phpShort is the dotless version (e.g. "85") and hash pins the
// image to the embedded Containerfile template.
func (l *Links) BaseImageRefs(phpShort, hash string) []string {
	suffix := "/lerd-php" + phpShort + "-fpm-base:" + hash
	if v := os.Getenv("LERD_BASE_IMAGE_REGISTRY"); v != "" {
		return []string{v + suffix}
	}
	return l.ordered(func(o org) string { return "ghcr.io/" + o.owner + suffix })
}

// NoteFetched flips to serving the new org first the first time a real request
// succeeds against a new-org URL (e.g. once the old org is retired). One-way,
// old->new only; no probe, no timer.
func (l *Links) NoteFetched(base string) {
	if base == "" || l.PreferNew() {
		return
	}
	if strings.Contains(base, "/"+l.new.owner+"/") {
		l.SetPreferNew(true)
	}
}

// splitList parses a comma-separated override into trimmed, non-empty entries.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Package-level helpers delegate to Default.
func StoreBaseURLs() []string                      { return Default.StoreBaseURLs() }
func ReleaseBaseURLs() []string                    { return Default.ReleaseBaseURLs() }
func ReleaseDownloadBases() []string               { return Default.ReleaseDownloadBases() }
func ReleaseAPIBaseURLs() []string                 { return Default.ReleaseAPIBaseURLs() }
func ChangelogURLs() []string                      { return Default.ChangelogURLs() }
func BaseImageRefs(phpShort, hash string) []string { return Default.BaseImageRefs(phpShort, hash) }
func NoteFetched(base string)                      { Default.NoteFetched(base) }
