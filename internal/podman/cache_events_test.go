package podman

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestRelevantEvent(t *testing.T) {
	cases := map[string]bool{
		"lerd-nginx\tstart":         true,
		"lerd-postgres\tdied":       true,
		"lerd-php85-fpm\tstop":      true,
		"lerd-php85-fpm\texec":      false, // workers exec constantly
		"lerd-php85-fpm\texec_died": false,
		"someone-else\tstart":       false,
		"lerd-nginx":                false,
		"":                          false,
	}
	for line, want := range cases {
		if got := relevantEvent(line); got != want {
			t.Errorf("relevantEvent(%q) = %v; want %v", line, got, want)
		}
	}
}

func TestEffectiveInterval(t *testing.T) {
	if got := effectiveInterval(15*time.Second, false); got != 15*time.Second {
		t.Errorf("without events got %v, want the chosen 15s", got)
	}
	if got := effectiveInterval(15*time.Second, true); got != eventsResync {
		t.Errorf("with live events got %v, want the %v resync", got, eventsResync)
	}
	if got := effectiveInterval(time.Hour, true); got != time.Hour {
		t.Errorf("a longer chosen interval must win, got %v", got)
	}
}

// A container event refreshes the cache at once, and the stream counts as live
// only while it is open, so polling falls back the moment it dies.
func TestWatchEvents_refreshesOnEventAndTracksLiveness(t *testing.T) {
	c := &ContainerCache{running: map[string]bool{}, refresh: make(chan struct{}, 1), interval: time.Second}
	pr, pw := io.Pipe()
	opened := make(chan struct{}, 1)
	c.eventsFn = func(ctx context.Context) (io.ReadCloser, error) {
		select {
		case opened <- struct{}{}:
			return pr, nil
		default:
			<-ctx.Done() // no second stream in this test
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.watchEvents(ctx)

	<-opened
	// Opening refreshes once, for whatever happened while no stream was open.
	select {
	case <-c.refresh:
	case <-time.After(2 * time.Second):
		t.Fatal("opening the stream did not refresh the cache")
	}
	// A pipe write returns only once the reader asks for more, which it does
	// after handling the line before, so each write proves the last was seen.
	write := func(line string) {
		if _, err := io.WriteString(pw, line+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	write("lerd-php85-fpm\texec")
	write("someone-else\tstart")
	select {
	case <-c.refresh:
		t.Fatal("an exec or foreign event refreshed the cache")
	default:
	}
	write("lerd-nginx\tdied")
	select {
	case <-c.refresh:
	case <-time.After(2 * time.Second):
		t.Fatal("a died event did not refresh the cache")
	}
	if !c.eventsLive.Load() {
		t.Error("stream open but not counted live")
	}
	pw.Close()
	// Closing wakes the poll loop, which may be sitting on the five minute
	// resync, so it falls back to its normal pace at once.
	select {
	case <-c.refresh:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the stream did not wake the poll loop")
	}
	deadline := time.Now().Add(2 * time.Second)
	for c.eventsLive.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if c.eventsLive.Load() {
		t.Error("stream closed but still counted live")
	}
}

// systemd is still "deactivating" when podman reports died, and a rebuild run
// then keeps that answer until its TTL, so a burst of events announces a change
// once more after it settles, whether or not the running map moved again.
func TestWatchEvents_announcesAgainOnceTheBurstSettles(t *testing.T) {
	old := eventsSettle
	eventsSettle = 50 * time.Millisecond
	t.Cleanup(func() { eventsSettle = old })

	c := &ContainerCache{running: map[string]bool{}, refresh: make(chan struct{}, 1), interval: time.Second}
	changes := make(chan struct{}, 10)
	c.SetOnChange(func() { changes <- struct{}{} })
	pr, pw := io.Pipe()
	c.eventsFn = func(ctx context.Context) (io.ReadCloser, error) { return pr, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.watchEvents(ctx)

	for _, l := range []string{"lerd-redis\tdied", "lerd-redis\tremove"} {
		if _, err := io.WriteString(pw, l+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-changes:
	case <-time.After(2 * time.Second):
		t.Fatal("no change announced after the burst settled")
	}
	select {
	case <-changes:
		t.Error("one burst announced more than once")
	case <-time.After(150 * time.Millisecond):
	}
}

// podman events can start, print why it cannot follow, and exit; that reason
// must reach the log once rather than vanish on every retry.
func TestStreamLogger_reportsEachDistinctFailureOnce(t *testing.T) {
	var got []string
	l := streamLogger{logf: func(format string, args ...any) { got = append(got, fmt.Sprintf(format, args...)) }}
	boom := errors.New("exit status 125: unsupported events backend")
	l.ended(boom)
	l.ended(boom)
	l.ended(nil)
	l.ended(errors.New("exit status 1"))
	if len(got) != 2 {
		t.Fatalf("logged %d times, want 2 (one per distinct failure): %q", len(got), got)
	}
}
