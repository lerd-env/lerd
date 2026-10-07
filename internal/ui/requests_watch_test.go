package ui

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/reqstats"
)

// quietRequestsFeed swaps the store watch for one that never starts, so a test
// drives the nudges itself.
func quietRequestsFeed(t *testing.T) {
	t.Helper()
	prev := requestsFeed
	requestsFeed = newChangeFeed(func(context.Context, func()) error { return nil })
	t.Cleanup(func() { requestsFeed = prev })
}

func TestRequestsWatchNudgesOnlyForANewerRequest(t *testing.T) {
	quietRequestsFeed(t)
	registerSite(t, "acme", "acme.test")
	now := time.Now().Truncate(time.Millisecond)
	store := seedAnalytics(t, []reqstats.Record{analyticsRecord("acme", "GET /", "/", now.Add(-time.Minute))})

	var w requestsWatch
	// The page loaded before it started watching, so a request in between would
	// be lost without one nudge as the watch begins.
	if w.watch("acme.test", "") == nil {
		t.Fatal("starting to watch did not nudge the page once")
	}
	defer w.stop()
	if _, ok := w.changed(); ok {
		t.Fatal("a request the page was already nudged for nudged it again")
	}

	if err := store.Insert([]reqstats.Record{analyticsRecord("acme", "GET /new", "/new", now)}); err != nil {
		t.Fatal(err)
	}
	frame, ok := w.changed()
	if !ok {
		t.Fatal("a new request on the watched site did not nudge the page")
	}
	var got map[string]string
	if err := json.Unmarshal(frame, &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "requests" || got["domain"] != "acme.test" {
		t.Errorf("frame = %s, want type requests for acme.test", frame)
	}
	if _, ok := w.changed(); ok {
		t.Error("the same request nudged the page twice")
	}
}

func TestRequestsWatchIgnoresOtherSites(t *testing.T) {
	quietRequestsFeed(t)
	registerSite(t, "acme", "acme.test")
	registerSite(t, "other", "other.test")
	now := time.Now()
	store := seedAnalytics(t, []reqstats.Record{analyticsRecord("acme", "GET /", "/", now.Add(-time.Minute))})

	var w requestsWatch
	w.watch("acme.test", "")
	defer w.stop()
	if err := store.Insert([]reqstats.Record{analyticsRecord("other", "GET /", "/", now)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.changed(); ok {
		t.Error("a request on another site nudged the watched one")
	}
}

func TestRequestsWatchLeavingTheSiteUnsubscribes(t *testing.T) {
	quietRequestsFeed(t)
	registerSite(t, "acme", "acme.test")

	var w requestsWatch
	w.watch("acme.test", "")
	if w.events() == nil || requestsFeed.subscribers() != 1 {
		t.Fatal("watching a site did not subscribe to the store feed")
	}
	w.watch("", "")
	if w.events() != nil || requestsFeed.subscribers() != 0 {
		t.Error("leaving the site kept the subscription")
	}
}

func TestRequestsWatchSeesASameMillisecondRequest(t *testing.T) {
	quietRequestsFeed(t)
	registerSite(t, "acme", "acme.test")
	at := time.Now().Truncate(time.Millisecond)
	store := seedAnalytics(t, []reqstats.Record{analyticsRecord("acme", "GET /a", "/a", at)})

	var w requestsWatch
	w.watch("acme.test", "")
	defer w.stop()
	if err := store.Insert([]reqstats.Record{analyticsRecord("acme", "GET /b", "/b", at)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.changed(); !ok {
		t.Error("a request in the same millisecond as the newest one went unnoticed")
	}
}

func TestChangeFeedRetriesAFailedStartAndStopsWhenIdle(t *testing.T) {
	starts := 0
	var running context.Context
	f := newChangeFeed(func(ctx context.Context, _ func()) error {
		starts++
		if starts == 1 {
			return errors.New("no directory yet")
		}
		running = ctx
		return nil
	})
	f.unsubscribe(f.subscribe())
	ch := f.subscribe()
	if starts != 2 || running == nil {
		t.Fatalf("starts = %d, want a retry after the first one failed", starts)
	}
	f.unsubscribe(ch)
	if running.Err() == nil {
		t.Error("the watch kept running with no page left on it")
	}
}

func TestChangeFeedNudgeNeverBlocks(t *testing.T) {
	f := newChangeFeed(func(context.Context, func()) error { return nil })
	ch := f.subscribe()
	f.notify()
	f.notify() // a page still busy with the first nudge must not stall the feed
	<-ch
	select {
	case <-ch:
		t.Error("two nudges before a read should collapse into one")
	default:
	}
	f.unsubscribe(ch)
	f.notify()
	select {
	case <-ch:
		t.Error("an unsubscribed channel was still nudged")
	default:
	}
}
