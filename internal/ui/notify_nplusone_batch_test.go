package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/push"
)

func workerWarning(site, command string) push.Notification {
	ev := qEvent("r", "", "select 1")
	ev.Ctx = dumps.Context{Type: "cli", Site: site, Command: command}
	return notificationForNPlusOne(ev, nPlusOneThreshold)
}

func TestNPlusOneBatch_LoneWarningPassesThrough(t *testing.T) {
	var got []push.Notification
	b := newNPlusOneBatch(time.Hour, func(n push.Notification) { got = append(got, n) })
	want := workerWarning("acme", "artisan sync:users")
	b.add(want)
	b.flush("acme")
	if len(got) != 1 || got[0].Body != want.Body || got[0].Tag != want.Tag {
		t.Fatalf("got %+v, want the original warning", got)
	}
}

// TestNPlusOneBatch_BurstBecomesOneNotification pins the parallel test runner
// case: every worker process has its own command line, so each trips its own
// warning, and the site should hear about it once.
func TestNPlusOneBatch_BurstBecomesOneNotification(t *testing.T) {
	var got []push.Notification
	b := newNPlusOneBatch(time.Hour, func(n push.Notification) { got = append(got, n) })
	for _, w := range []string{"worker.php --status-file /tmp/worker_03", "worker.php --status-file /tmp/worker_16", "worker.php --status-file /tmp/worker_29"} {
		b.add(workerWarning("acme", w))
	}
	b.add(workerWarning("other", "artisan import"))
	b.flush("acme")
	b.flush("other")

	if len(got) != 2 {
		t.Fatalf("got %d notifications, want one per site", len(got))
	}
	g := got[0]
	if g.Title != "Possible N+1 queries on acme" {
		t.Errorf("title = %q", g.Title)
	}
	if !strings.HasPrefix(g.Body, "3 runs repeated a similar query") || !strings.Contains(g.Body, "worker_03") {
		t.Errorf("body = %q", g.Body)
	}
	if g.Tag != "lerd-nplusone-site-acme" || g.Kind != "nplusone" {
		t.Errorf("tag/kind = %q/%q", g.Tag, g.Kind)
	}
	if got[1].Title != "Possible N+1 query on other" {
		t.Errorf("other site should pass through, got %q", got[1].Title)
	}
}

func TestNPlusOneBatch_FlushesAfterWindow(t *testing.T) {
	got := make(chan push.Notification, 4)
	b := newNPlusOneBatch(10*time.Millisecond, func(n push.Notification) { got <- n })
	b.add(workerWarning("acme", "worker.php 1"))
	b.add(workerWarning("acme", "worker.php 2"))
	select {
	case n := <-got:
		if !strings.HasPrefix(n.Body, "2 runs") {
			t.Errorf("body = %q", n.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("window never flushed")
	}
	if len(got) != 0 {
		t.Errorf("%d extra notifications after the grouped one", len(got))
	}
}
