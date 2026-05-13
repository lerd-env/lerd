package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	out := make(chan tea.Msg, 8)
	go func() {
		msg := cmd()
		if msg == nil {
			out <- nil
			return
		}
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				c := c
				go func() { out <- c() }()
			}
		default:
			out <- v
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
