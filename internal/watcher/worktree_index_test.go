package watcher

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/reqstats"
)

// seedSiteWithWorktree registers one site and stands in a single worktree for it,
// swapping in a private index so a test never leans on the daemon's.
func seedSiteWithWorktree(t *testing.T, wts []gitpkg.Worktree, detectErr error) *worktreeIndex {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.AddSite(config.Site{
		Name: "myapp", Path: "/srv/myapp", PHPVersion: "8.4", Domains: []string{"myapp.test"},
	}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	prevDetect, prevIndex := detectWorktrees, wtIndex
	detectWorktrees = func(string, string) ([]gitpkg.Worktree, error) { return wts, detectErr }
	wtIndex = newWorktreeIndex()
	t.Cleanup(func() { detectWorktrees, wtIndex = prevDetect, prevIndex })
	return wtIndex
}

// A worktree's requests must be attributed with idle-suspend off, which is the
// default. The stats key is the sanitized branch, the identity the HTTP API and
// the store share, while the idle key is the checkout dir's unit slug, which the
// worker units use. Resolution used to run through the idle engine's map, built
// only while it ticked, so with the feature disabled a worktree request resolved
// to nothing and its record was dropped.
func TestResolveHost_WorktreeWithIdleSuspendOff(t *testing.T) {
	idx := seedSiteWithWorktree(t, []gitpkg.Worktree{{
		Branch: "feature-auth",
		Path:   "/srv/myapp/myapp-feature-auth",
		Domain: "feature-auth.myapp.test",
	}}, nil)
	idx.refresh() // no engine, no tick: the index stands alone

	got, ok := resolveHostToStatsKey("feature-auth.myapp.test")
	if !ok || got != reqstats.Key("myapp", "feature-auth") {
		t.Errorf("stats key = %q (ok=%v), want %q", got, ok, reqstats.Key("myapp", "feature-auth"))
	}
	idleKey, ok := resolveHostToSite("feature-auth.myapp.test")
	if !ok || idleKey != wtKey("myapp", "myapp-feature-auth") {
		t.Errorf("idle key = %q (ok=%v), want %q", idleKey, ok, wtKey("myapp", "myapp-feature-auth"))
	}

	// The parent still resolves to its own name under both schemes.
	if got, ok := resolveHostToStatsKey("myapp.test"); !ok || got != "myapp" {
		t.Errorf("parent stats key = %q (ok=%v), want %q", got, ok, "myapp")
	}
	// An unregistered host belongs to no site and must stay unresolved.
	if _, ok := resolveHostToStatsKey("nope.test"); ok {
		t.Error("unregistered host resolved to a site")
	}
}

// A transient git error must not unresolve a live worktree: dropping it from the
// index would silently discard every request it serves until detection recovers.
func TestWorktreeIndex_KeepsLastGoodViewOnDetectError(t *testing.T) {
	idx := seedSiteWithWorktree(t, []gitpkg.Worktree{{
		Branch: "feature-auth",
		Path:   "/srv/myapp/myapp-feature-auth",
		Domain: "feature-auth.myapp.test",
	}}, nil)
	idx.refresh()

	detectWorktrees = func(string, string) ([]gitpkg.Worktree, error) {
		return nil, errors.New("git exploded")
	}
	idx.refresh()

	ref, ok := idx.lookup("feature-auth.myapp.test")
	if !ok {
		t.Fatal("worktree dropped from the index on a transient detect error")
	}
	if ref.Branch != "feature-auth" || ref.Path != "/srv/myapp/myapp-feature-auth" {
		t.Errorf("retained ref = %+v, want the last good view", ref)
	}
}

// A worktree whose subdomain is reserved by a group secondary is served by that
// secondary's vhost, so its host must resolve to the secondary's site rather than
// to the worktree. The worktree stays in the per-site view, since idle-suspend
// still owns its workers.
func TestWorktreeIndex_ReservedSubdomainResolvesToTheSecondary(t *testing.T) {
	idx := seedSiteWithWorktree(t, []gitpkg.Worktree{{
		Branch: "admin",
		Path:   "/srv/myapp/myapp-admin",
		Domain: "admin.myapp.test",
	}}, nil)
	if err := config.AddSite(config.Site{
		Name: "myapp-admin", Path: "/srv/admin", PHPVersion: "8.4",
		Domains: []string{"admin.myapp.test"}, Group: "myapp", GroupSubdomain: "admin",
	}); err != nil {
		t.Fatalf("seed secondary: %v", err)
	}
	idx.refresh()

	if _, ok := idx.lookup("admin.myapp.test"); ok {
		t.Error("reserved subdomain resolved to the worktree, stealing the secondary's traffic")
	}
	if got, ok := resolveHostToStatsKey("admin.myapp.test"); !ok || got != "myapp-admin" {
		t.Errorf("stats key = %q (ok=%v), want the secondary site %q", got, ok, "myapp-admin")
	}
	if len(idx.forSite("myapp")) != 1 {
		t.Error("reserved worktree dropped from the per-site view, so idle-suspend would stop tracking it")
	}
}
