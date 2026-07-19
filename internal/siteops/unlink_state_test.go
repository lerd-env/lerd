package siteops

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/idle"
	"github.com/geodro/lerd/internal/reqstats"
)

// TestForgetSiteState_purgesDurableState proves the unlink path clears an
// unlinked site's request-timing and idle state: both snapshot files and the
// durable SQLite store, covering the site's worktree keys too.
func TestForgetSiteState_purgesDurableState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	reqstats.SaveSnapshot([]reqstats.SiteStats{ //nolint:errcheck
		{Site: "gone"}, {Site: "gone/feature-x"}, {Site: "keep"},
	}, config.RequestStatsFile())

	tr := idle.NewTracker(nil)
	tr.TouchSite("gone", time.Unix(1000, 0))
	tr.TouchSite("gone/feature-x", time.Unix(1000, 0))
	tr.TouchSite("keep", time.Unix(1000, 0))
	tr.Save(config.IdleActivityFile()) //nolint:errcheck

	store, err := reqstats.OpenStore(config.RequestStatsDB())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	store.Insert([]reqstats.Record{ //nolint:errcheck
		{At: time.Unix(1000, 0), Site: "gone", Route: "GET /a", Method: "GET", Status: 200, Millis: 20, URI: "/a"},
		{At: time.Unix(1000, 0), Site: "gone/feature-x", Route: "GET /b", Method: "GET", Status: 200, Millis: 20, URI: "/b"},
		{At: time.Unix(1000, 0), Site: "keep", Route: "GET /c", Method: "GET", Status: 200, Millis: 20, URI: "/c"},
	})
	store.Close()

	forgetSiteState("gone")

	if _, ok := reqstats.LoadSite(config.RequestStatsFile(), "gone"); ok {
		t.Error("stats file still carries the unlinked site")
	}
	if _, ok := reqstats.LoadSite(config.RequestStatsFile(), "keep"); !ok {
		t.Error("stats file lost an unrelated site")
	}
	if m := idle.LoadActivity(config.IdleActivityFile()); m["gone"] != 0 || m["gone/feature-x"] != 0 {
		t.Errorf("idle file still carries the unlinked site: %+v", m)
	}

	store2, err := reqstats.OpenStore(config.RequestStatsDB())
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()
	a, _ := store2.SiteAnalytics("gone", time.Unix(0, 0), time.Unix(1<<31, 0))
	if a.Samples != 0 {
		t.Errorf("durable store still has %d rows for the unlinked site", a.Samples)
	}
	keep, _ := store2.SiteAnalytics("keep", time.Unix(0, 0), time.Unix(1<<31, 0))
	if keep.Samples != 1 {
		t.Errorf("durable store lost an unrelated site's rows: %d", keep.Samples)
	}
}
