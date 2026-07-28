package dns

import (
	"testing"
	"time"
)

// TestDebounceEvents pins the burst-collapsing behaviour. A single VPN
// connect fires a flurry of RTM_NEWLINK / RTM_NEWADDR messages, and any
// reactive re-sync must not run mid-burst against an intermediate host
// resolver state. A burst should produce exactly one settled event after
// it quiets; well-separated events each produce their own.
func TestDebounceEvents(t *testing.T) {
	t.Run("single event emits once after wait", func(t *testing.T) {
		in := make(chan struct{}, 8)
		out := make(chan struct{}, 8)
		done := make(chan struct{})
		stopped := make(chan struct{})
		go func() { DebounceEvents(in, out, 40*time.Millisecond, done); close(stopped) }()
		defer func() { close(done); <-stopped }()

		in <- struct{}{}
		select {
		case <-out:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("settled event never emitted")
		}
		select {
		case <-out:
			t.Fatal("unexpected second emit from a single input")
		case <-time.After(120 * time.Millisecond):
		}
	})

	t.Run("burst collapses to one emit", func(t *testing.T) {
		in := make(chan struct{}, 64)
		out := make(chan struct{}, 64)
		done := make(chan struct{})
		stopped := make(chan struct{})
		// The window has to be wide enough that a scheduling stall between two
		// sends cannot exceed it. A 10ms gap against a 60ms window left only six
		// times the margin, and a loaded CI runner overshot a sleep far enough to
		// settle mid-burst and emit twice.
		go func() { DebounceEvents(in, out, 400*time.Millisecond, done); close(stopped) }()
		defer func() { close(done); <-stopped }()

		for i := 0; i < 10; i++ {
			in <- struct{}{}
			time.Sleep(5 * time.Millisecond)
		}
		emits := 0
		// Long enough for the settled emit to land, and to catch a second one.
		timeout := time.After(1500 * time.Millisecond)
	collect:
		for {
			select {
			case <-out:
				emits++
			case <-timeout:
				break collect
			}
		}
		if emits != 1 {
			t.Fatalf("10-event burst produced %d settled emits, want 1", emits)
		}
	})

	t.Run("well separated events each emit", func(t *testing.T) {
		in := make(chan struct{}, 8)
		out := make(chan struct{}, 8)
		done := make(chan struct{})
		stopped := make(chan struct{})
		go func() { DebounceEvents(in, out, 40*time.Millisecond, done); close(stopped) }()
		defer func() { close(done); <-stopped }()

		in <- struct{}{}
		select {
		case <-out:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("first emit missing")
		}
		time.Sleep(60 * time.Millisecond)
		in <- struct{}{}
		select {
		case <-out:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("second emit missing")
		}
	})

	t.Run("done channel stops the goroutine", func(t *testing.T) {
		in := make(chan struct{}, 8)
		out := make(chan struct{}, 8)
		done := make(chan struct{})
		stopped := make(chan struct{})
		go func() { DebounceEvents(in, out, 40*time.Millisecond, done); close(stopped) }()

		close(done)
		select {
		case <-stopped:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("DebounceEvents did not return on done close")
		}
	})
}
