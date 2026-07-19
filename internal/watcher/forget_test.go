package watcher

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/idle"
	"github.com/geodro/lerd/internal/reqstats"
)

// TestForgetSiteState_dropsInMemoryAndFiles proves a "forget <site>" control
// line drops the site (and its worktree keys) from the live request aggregator
// and idle tracker and re-persists both snapshot files, so a running watcher
// stops re-emitting an unlinked site into the state files.
func TestForgetSiteState_dropsInMemoryAndFiles(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	prevTracker, prevAgg := activityTracker, reqAggregator
	t.Cleanup(func() { activityTracker = prevTracker; reqAggregator = prevAgg })

	activityTracker = idle.NewTracker(nil)
	activityTracker.TouchSite("gone", time.Unix(1000, 0))
	activityTracker.TouchSite("gone/feature-x", time.Unix(1000, 0))
	activityTracker.TouchSite("keep", time.Unix(1000, 0))

	reqAggregator = reqstats.New(func(host string) (string, bool) {
		switch host {
		case "gone.test":
			return "gone", true
		case "keep.test":
			return "keep", true
		}
		return "", false
	})
	for i := 0; i < 10; i++ {
		reqAggregator.Record(reqstats.AccessRecord{Host: "gone.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: "/home"})
		reqAggregator.Record(reqstats.AccessRecord{Host: "keep.test", Status: 200, RequestTime: 0.04, Method: "GET", URI: "/home"})
	}

	dispatchControl("forget gone")

	if _, ok := activityTracker.LastActive("gone"); ok {
		t.Error("forget must drop the site from the idle tracker")
	}
	if _, ok := activityTracker.LastActive("gone/feature-x"); ok {
		t.Error("forget must drop the site's worktree from the idle tracker")
	}
	if _, ok := activityTracker.LastActive("keep"); !ok {
		t.Error("forget must not touch an unrelated site")
	}
	if _, ok := reqAggregator.SiteSnapshot("gone"); ok {
		t.Error("forget must drop the site from the request aggregator")
	}

	if _, ok := reqstats.LoadSite(config.RequestStatsFile(), "gone"); ok {
		t.Error("forget must rewrite the stats file without the site")
	}
	if _, ok := reqstats.LoadSite(config.RequestStatsFile(), "keep"); !ok {
		t.Error("forget must leave an unrelated site in the stats file")
	}
	if m := idle.LoadActivity(config.IdleActivityFile()); m["gone"] != 0 || m["keep"] == 0 {
		t.Errorf("forget must rewrite the idle file without the site: %+v", m)
	}
}
