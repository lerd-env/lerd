package cli

import "testing"

// TestWorkerNameForSiteUnit covers the parsing the prune sweep relies on to
// decide which units belong to a site, including the prefix-collision case
// (site "app" vs "app-x") that the longest-match guard in stopAllSiteWorkerUnits
// uses to avoid tearing down another site's units.
func TestWorkerNameForSiteUnit(t *testing.T) {
	cases := []struct {
		unit, site string
		wantWorker string
		wantOK     bool
	}{
		{"lerd-queue-app", "app", "queue", true},               // parent unit
		{"lerd-vite-app-feat", "app", "vite", true},            // worktree unit
		{"lerd-queue-other", "app", "", false},                 // different site
		{"lerd-vite-app-x", "app-x", "vite", true},             // parent unit of the longer-named site
		{"lerd-vite-app-x", "app", "vite", true},               // also matches "app" as a worktree (slug "x")
		{"lerd-app", "app", "", false},                         // no worker segment
		{"queue-app", "app", "", false},                        // missing lerd- prefix
		{"lerd-messenger-my-app", "my-app", "messenger", true}, // hyphenated site name
	}
	for _, c := range cases {
		gotWorker, gotOK := workerNameForSiteUnit(c.unit, c.site)
		if gotOK != c.wantOK || gotWorker != c.wantWorker {
			t.Errorf("workerNameForSiteUnit(%q, %q) = (%q, %v), want (%q, %v)",
				c.unit, c.site, gotWorker, gotOK, c.wantWorker, c.wantOK)
		}
	}
}

// TestSiteOwnsWorkerUnit_declinesAmbiguous guards against tearing down another
// site's unit: unlinking a site named "feat" must NOT claim site "web"'s
// "lerd-horizon-web-feat" worktree unit, because "web" is also a registered site.
func TestSiteOwnsWorkerUnit_declinesAmbiguous(t *testing.T) {
	// "feat" parses lerd-horizon-web-feat as its own (worker "horizon-web"), but
	// "web" is a registered site that also parses it, so ownership is declined.
	if _, ok := siteOwnsWorkerUnit("lerd-horizon-web-feat", "feat", []string{"web"}); ok {
		t.Error("site \"feat\" must not claim web's worktree unit lerd-horizon-web-feat")
	}
	// With no colliding site registered, "feat" legitimately owns its own units.
	if _, ok := siteOwnsWorkerUnit("lerd-queue-feat", "feat", []string{"web"}); !ok {
		t.Error("site \"feat\" should own its own parent unit lerd-queue-feat")
	}
	// A site cleanly owns its worktree unit when no other site collides.
	if _, ok := siteOwnsWorkerUnit("lerd-vite-app-feat", "app", []string{"web"}); !ok {
		t.Error("site \"app\" should own its worktree unit lerd-vite-app-feat")
	}
}
