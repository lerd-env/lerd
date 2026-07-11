package watcher

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// worktreeIndexInterval is how often the index re-detects worktrees. Worktrees
// added or removed through the daemon refresh it immediately (RefreshWorktreeIndex),
// so this only bounds how long a worktree created behind lerd's back stays unknown.
const worktreeIndexInterval = 30 * time.Second

// worktreeRef is one detected worktree in both the identities lerd gives it.
// Branch is the sanitized branch, which the HTTP API, MCP and the request store
// key on; Base is the checkout dir's unit slug, which worker units and idle
// state key on. Keeping both here is what lets one detection pass serve the
// request-timing resolver and the idle engine without either guessing the other's
// spelling.
type worktreeRef struct {
	Site   string
	Branch string
	Base   string
	Path   string
	Domain string
}

// worktreeIndex is the daemon's view of every site's worktrees, keyed by domain.
// It refreshes for the daemon's whole life rather than inside an idle-suspend
// session, because request timing must attribute worktree traffic with
// idle-suspend off; the engine's own map was only ever built while it ticked, so
// with the feature disabled every worktree request resolved to no site and was
// dropped.
type worktreeIndex struct {
	mu     sync.RWMutex
	byHost map[string]worktreeRef
	bySite map[string][]worktreeRef
}

func newWorktreeIndex() *worktreeIndex {
	return &worktreeIndex{byHost: map[string]worktreeRef{}, bySite: map[string][]worktreeRef{}}
}

// wtIndex is the shared index, never nil so a lookup before the first refresh
// answers empty rather than panicking.
var wtIndex = newWorktreeIndex()

// RefreshWorktreeIndex re-detects worktrees now. The daemon calls it whenever a
// worktree is added, changed or removed, so a new worktree's traffic is
// attributed from its first request rather than from the next scheduled refresh.
func RefreshWorktreeIndex() { wtIndex.refresh() }

// refresh re-detects every site's worktrees. A site whose detection fails keeps
// its previous entries: dropping them on a transient git error would make a live
// worktree unresolvable and silently discard its requests.
//
// Host lookups see only the servable worktrees, matching what nginx serves, so a
// subdomain reserved by a group secondary resolves to that secondary rather than
// to a same-named worktree that has no vhost.
func (x *worktreeIndex) refresh() {
	reg, err := config.LoadSites()
	if err != nil {
		return
	}
	byHost := map[string]worktreeRef{}
	bySite := map[string][]worktreeRef{}
	for i := range reg.Sites {
		s := reg.Sites[i]
		wts, err := detectWorktrees(s.Path, s.PrimaryDomain())
		if err != nil {
			bySite[s.Name] = x.forSite(s.Name)
			for host, ref := range x.hosts() {
				if ref.Site == s.Name {
					byHost[host] = ref
				}
			}
			continue
		}
		for _, wt := range wts {
			bySite[s.Name] = append(bySite[s.Name], refFor(s.Name, wt))
		}
		for _, wt := range gitpkg.FilterReservedWorktrees(wts) {
			if wt.Domain != "" {
				byHost[strings.ToLower(wt.Domain)] = refFor(s.Name, wt)
			}
		}
	}
	x.mu.Lock()
	x.byHost, x.bySite = byHost, bySite
	x.mu.Unlock()
}

// hosts returns a copy of the current domain lookup, so a refresh can carry a
// site's entries forward without holding the lock across detection.
func (x *worktreeIndex) hosts() map[string]worktreeRef {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make(map[string]worktreeRef, len(x.byHost))
	for host, ref := range x.byHost {
		out[host] = ref
	}
	return out
}

// refFor builds the index entry for one detected worktree of a site.
func refFor(site string, wt gitpkg.Worktree) worktreeRef {
	return worktreeRef{
		Site:   site,
		Branch: wt.Branch,
		Base:   config.WorktreeUnitSlug(filepath.Base(wt.Path)),
		Path:   wt.Path,
		Domain: wt.Domain,
	}
}

// run refreshes the index on a ticker for as long as the daemon runs.
func (x *worktreeIndex) run() {
	t := time.NewTicker(worktreeIndexInterval)
	defer t.Stop()
	for range t.C {
		x.refresh()
	}
}

// lookup resolves a request host to the worktree it is served from.
func (x *worktreeIndex) lookup(host string) (worktreeRef, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	ref, ok := x.byHost[strings.ToLower(host)]
	return ref, ok
}

// forSite returns the site's detected worktrees, including any whose subdomain is
// reserved: idle-suspend still owns their workers even though nothing serves them.
func (x *worktreeIndex) forSite(site string) []worktreeRef {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.bySite[site]
}

// pathFor resolves a worktree's checkout dir from the base its idle key carries,
// "" when no such worktree is known.
func (x *worktreeIndex) pathFor(site, base string) string {
	for _, ref := range x.forSite(site) {
		if ref.Base == base {
			return ref.Path
		}
	}
	return ""
}

// domainFor resolves a worktree's domain from the branch its store key carries,
// "" when no such worktree is known.
func (x *worktreeIndex) domainFor(site, branch string) string {
	for _, ref := range x.forSite(site) {
		if ref.Branch == branch {
			return ref.Domain
		}
	}
	return ""
}
