package tui

import (
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/eventbus"
)

// TestBusMsg_rechainsBusCmd pins the regression-prone wiring where Update
// must re-chain busCmd after every bus publish. Without the re-chain the
// in-process eventbus only ever drives the TUI once per process lifetime,
// because tea.Cmd is one-shot.
//
// The test runs two publishes back to back and asserts both reach the
// Update handler as busMsg.
func TestBusMsg_rechainsBusCmd(t *testing.T) {
	hub := eventbus.New()
	hub.SetDebounce(1 * time.Millisecond)
	sub := hub.Subscribe()
	t.Cleanup(func() { hub.Unsubscribe(sub) })

	m := &Model{sub: sub}

	// Drive the busCmd manually the first time so we don't depend on
	// model.Init() (which kicks off goroutines we don't need here).
	cmd := busCmd(sub)

	hub.Publish(eventbus.KindSites)
	msg := runCmdWithTimeout(t, cmd, time.Second)
	if _, ok := msg.(busMsg); !ok {
		t.Fatalf("first publish: got %T, want busMsg", msg)
	}

	// Hand busMsg to Update. The returned Cmd must include a re-chained
	// busCmd; if it doesn't, a second publish will never deliver.
	_, next := m.Update(msg)
	if next == nil {
		t.Fatalf("Update(busMsg) returned nil cmd; expected loadCmd batched with re-chained busCmd")
	}

	// Publish again; the next Cmd should produce a second busMsg via the
	// re-chained subscription. We Batch loadCmd + busCmd, so execute the
	// batch and look for the busMsg specifically.
	hub.Publish(eventbus.KindServices)
	got := runBatchUntil[busMsg](t, next, 2*time.Second)
	if got == nil {
		t.Fatal("second publish never reached Update; busCmd was not re-chained")
	}
}

// runCmdWithTimeout executes a tea.Cmd in a goroutine and returns the
// emitted message, or fails the test on timeout.
func runCmdWithTimeout(t *testing.T, cmd tea.Cmd, d time.Duration) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case m := <-out:
		return m
	case <-time.After(d):
		t.Fatal("cmd did not return in time")
		return nil
	}
}

// runBatchUntil walks a (possibly batched) tea.Cmd and returns the first
// emitted message whose type matches T. Returns the zero value pointer if
// no such message arrives within d.
func runBatchUntil[T any](t *testing.T, cmd tea.Cmd, d time.Duration) *T {
	t.Helper()
	if cmd == nil {
		return nil
	}
	// A command runs the real TUI load path, which touches the site registry, so
	// don't return while the rest of the batch is still in it. The wait is bounded:
	// some commands (busCmd) park until a cleanup registered before this one closes
	// their channel, and cleanups run last-registered-first, so an unbounded wait
	// here would hang the package instead of failing a test.
	var wg sync.WaitGroup
	t.Cleanup(func() { waitBounded(&wg, 5*time.Second) })

	out := make(chan tea.Msg, 64)
	send := func(m tea.Msg) { // never block a goroutine nobody is reading any more
		select {
		case out <- m:
		default:
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := cmd()
		if msg == nil {
			send(nil)
			return
		}
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				wg.Add(1)
				go func() {
					defer wg.Done()
					send(c())
				}()
			}
		default:
			send(v)
		}
	}()
	deadline := time.After(d)
	for {
		select {
		case m := <-out:
			if got, ok := m.(T); ok {
				return &got
			}
		case <-deadline:
			return nil
		}
	}
}

// waitBounded waits for wg, giving up after d. A command parked on a channel that
// only a later cleanup closes must not hang the whole package.
func waitBounded(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
	}
}
