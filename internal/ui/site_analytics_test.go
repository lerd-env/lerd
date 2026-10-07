package ui

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

func TestAnalyticsRange(t *testing.T) {
	cases := map[string]struct {
		dur   time.Duration
		label string
	}{
		"15m":  {15 * time.Minute, "15m"},
		"1h":   {time.Hour, "1h"},
		"24h":  {24 * time.Hour, "24h"},
		"7d":   {7 * 24 * time.Hour, "7d"},
		"":     {time.Hour, "1h"}, // absent falls back to 1h
		"nope": {time.Hour, "1h"}, // unknown falls back to 1h
	}
	for in, want := range cases {
		dur, label := analyticsRange(in)
		if dur != want.dur || label != want.label {
			t.Errorf("analyticsRange(%q) = %v/%q, want %v/%q", in, dur, label, want.dur, want.label)
		}
	}
}

// Inspect shows only on a request whose captured events are still buffered.
func TestRecentRowsKeepTheIDOnlyWithCapturedEvents(t *testing.T) {
	rows := recentRows([]reqstats.Record{{URI: "/a", RID: "r1"}, {URI: "/b", RID: "r2"}, {URI: "/c"}}, map[string]bool{"r1": true}, map[string]string{"r2": "spx-full-2"})
	if rows[0].RID != "r1" || rows[1].RID != "" || rows[2].RID != "" {
		t.Fatalf("rids = %q %q %q", rows[0].RID, rows[1].RID, rows[2].RID)
	}
	// A flame graph outlives the debug events: r2's are gone, its capture is not.
	if rows[0].ProfileKey != "" || rows[1].ProfileKey != "spx-full-2" {
		t.Fatalf("profile keys = %q %q", rows[0].ProfileKey, rows[1].ProfileKey)
	}
}

func TestLinkSlowestKeepsWhatIsLeftOfEachRequest(t *testing.T) {
	routes := []reqstats.RouteStat{{Slowest: &reqstats.Sample{RID: "r1"}}, {Slowest: &reqstats.Sample{RID: "r2"}}, {}}
	linkSlowest(routes, map[string]bool{"r1": true}, map[string]string{"r2": "spx-full-2"})
	if routes[0].Slowest.RID != "r1" || routes[1].Slowest.RID != "" {
		t.Fatalf("rids = %q %q", routes[0].Slowest.RID, routes[1].Slowest.RID)
	}
	if routes[1].Slowest.ProfileKey != "spx-full-2" {
		t.Fatalf("profile key = %q", routes[1].Slowest.ProfileKey)
	}
}

// Recent requests page by count: 20 by default, capped so one request stays cheap.
func TestRecentLimit(t *testing.T) {
	for in, want := range map[string]int{"": 20, "40": 40, "0": 20, "-5": 20, "abc": 20, "100000": 500} {
		if got := recentLimit(in); got != want {
			t.Errorf("recentLimit(%q) = %d, want %d", in, got, want)
		}
	}
}

// At the 500 cap there is no further page to ask for, so it never says there is.
func TestRecentPageStopsAtTheCap(t *testing.T) {
	if pageHasMore(501, 500) {
		t.Fatal("a capped page must not offer more")
	}
	if !pageHasMore(21, 20) || pageHasMore(20, 20) {
		t.Fatal("below the cap, one row past the page means more")
	}
}
