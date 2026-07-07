package watcher

import (
	"testing"

	"github.com/geodro/lerd/internal/reqstats"
)

func snapWithSlow(routes ...string) []reqstats.SiteStats {
	slow := make([]reqstats.RouteStat, 0, len(routes))
	for _, r := range routes {
		slow = append(slow, reqstats.RouteStat{Route: r, Method: "GET", P95Millis: 900, Multiplier: 12})
	}
	return []reqstats.SiteStats{{Site: "acme", MedianMillis: 40, Slow: slow}}
}

func idDomain(site string) string { return site + ".test" }

func TestSlowRouteNotifier_firesOncePerRoute(t *testing.T) {
	n := newSlowRouteNotifier()

	first := n.notifications(snapWithSlow("GET /reports/:id"), idDomain)
	if len(first) != 1 {
		t.Fatalf("first pass: got %d notifications, want 1", len(first))
	}
	if first[0].Kind != "slow_route" || first[0].URL != "#sites/acme.test/dumps" {
		t.Errorf("notification = %+v", first[0])
	}

	// Same route still flagged on the next tick must not notify again.
	second := n.notifications(snapWithSlow("GET /reports/:id"), idDomain)
	if len(second) != 0 {
		t.Errorf("second pass: got %d notifications, want 0 (already warned)", len(second))
	}

	// A newly-flagged route does notify, the old one stays silent.
	third := n.notifications(snapWithSlow("GET /reports/:id", "GET /export"), idDomain)
	if len(third) != 1 || third[0].Data["route"] != "GET /export" {
		t.Errorf("third pass: want only the new /export route, got %+v", third)
	}
}

func TestSlowRouteNotifier_renotifiesAfterRecovery(t *testing.T) {
	n := newSlowRouteNotifier()

	if got := n.notifications(snapWithSlow("GET /reports/:id"), idDomain); len(got) != 1 {
		t.Fatalf("initial slowdown should notify, got %d", len(got))
	}
	// Route drops back within typical (absent from Slow) for a sustained run of
	// snapshots: only then is the warned state cleared.
	empty := []reqstats.SiteStats{{Site: "acme", MedianMillis: 40}}
	for i := 0; i < slowRouteClearAfter; i++ {
		if got := n.notifications(empty, idDomain); len(got) != 0 {
			t.Fatalf("recovery must not notify, got %d", len(got))
		}
	}
	// Slow again: since it recovered, it notifies afresh.
	if got := n.notifications(snapWithSlow("GET /reports/:id"), idDomain); len(got) != 1 {
		t.Errorf("a route that recovered then slowed again must re-notify, got %d", len(got))
	}
}

// A still-slow route can be bumped off the truncated Slow list by slower siblings
// for a snapshot or two, then reappear. That brief displacement must not be read
// as a recovery, or it fires a duplicate notification (finding #3).
func TestSlowRouteNotifier_noRenotifyOnBriefDisplacement(t *testing.T) {
	n := newSlowRouteNotifier()

	if got := n.notifications(snapWithSlow("GET /reports/:id"), idDomain); len(got) != 1 {
		t.Fatalf("initial slowdown should notify, got %d", len(got))
	}
	// Absent for fewer than the clear threshold (displaced, not recovered).
	empty := []reqstats.SiteStats{{Site: "acme", MedianMillis: 40}}
	for i := 0; i < slowRouteClearAfter-1; i++ {
		if got := n.notifications(empty, idDomain); len(got) != 0 {
			t.Fatalf("displacement must not notify, got %d", len(got))
		}
	}
	// Reappears while still within the grace window: must stay silent.
	if got := n.notifications(snapWithSlow("GET /reports/:id"), idDomain); len(got) != 0 {
		t.Errorf("a route briefly displaced from the top list must not re-notify, got %d", len(got))
	}
}

// A route flagged only in absolute terms (no usable site baseline, multiplier 0)
// must read as its p95, not "0x slower than usual" (finding #4).
func TestNotificationForSlowRoute_zeroMultiplierUsesAbsolute(t *testing.T) {
	r := reqstats.RouteStat{Route: "GET /export", Method: "GET", P95Millis: 1200, Multiplier: 0}
	body := notificationForSlowRoute("acme", "acme.test", r).Body
	if want := "GET /export p95 is 1200ms"; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	r.Multiplier = 5
	if body := notificationForSlowRoute("acme", "acme.test", r).Body; body == "GET /export p95 is 1200ms" {
		t.Errorf("with a real multiplier the body should use the slower-than-usual phrasing, got %q", body)
	}
}
