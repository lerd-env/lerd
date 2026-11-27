package dumps

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func floodEvent(t *testing.T, i int) string {
	return mustJSON(t, Event{
		V: 1, ID: "e" + strconv.Itoa(i), TS: "2026-05-10T00:00:00.000Z", Kind: "dump",
		Ctx: Context{Type: "cli", Site: "x"},
	})
}

func (s *Server) openConns() int {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return len(s.conns)
}

// A bridge connection hands its events over and is done even while the buffer
// is busy. Connections queued on the buffer's lock, each holding its read
// buffer, are what grew lerd-ui to gigabytes under a PHP flood.
func TestServer_ConnectionsDoNotWaitOnABusyBuffer(t *testing.T) {
	s := startServer(t)
	s.ring.mu.Lock()
	unlocked := false
	t.Cleanup(func() {
		if !unlocked {
			s.ring.mu.Unlock()
		}
	})
	for i := range 20 {
		dialAndSend(t, s.Addr(), floodEvent(t, i))
	}
	if !waitFor(func() bool { return s.openConns() == 0 }, 2*time.Second) {
		t.Fatalf("%d connections still waiting on the buffer", s.openConns())
	}
	s.ring.mu.Unlock()
	unlocked = true
	if !waitFor(func() bool { return s.Len() == 20 }, 2*time.Second) {
		t.Fatalf("buffered %d of 20 events once the buffer was free", s.Len())
	}
}

// When events arrive faster than the buffer stores them, the excess is dropped
// rather than held: memory stays bounded however hard PHP is hit.
func TestServer_FullQueueDropsInsteadOfGrowing(t *testing.T) {
	prev := queueSize
	queueSize = 4
	t.Cleanup(func() { queueSize = prev })
	s := startServer(t)
	s.ring.mu.Lock()
	var lines strings.Builder
	for i := range 200 {
		lines.WriteString(floodEvent(t, i))
	}
	dialAndSend(t, s.Addr(), lines.String())
	if !waitFor(func() bool { return s.openConns() == 0 }, 2*time.Second) {
		s.ring.mu.Unlock()
		t.Fatalf("the connection waited on a full queue")
	}
	s.ring.mu.Unlock()
	waitFor(func() bool { return s.Len() > 0 }, 2*time.Second)
	time.Sleep(100 * time.Millisecond)
	if n := s.Len(); n == 0 || n >= 200 {
		t.Errorf("buffered %d of 200 events, want some kept and the excess dropped", n)
	}
}

// The queue is bounded in bytes too: a few large dumps waiting to be stored
// must not hold more memory than the budget.
func TestServer_QueueHoldsNoMoreThanItsByteBudget(t *testing.T) {
	prev := queueBytes
	queueBytes = 10 << 10
	t.Cleanup(func() { queueBytes = prev })
	reached, release := holdWriter(t)
	s := startServer(t)
	t.Cleanup(release)
	big := strings.Repeat("x", 4<<10)
	var lines strings.Builder
	for i := range 20 {
		lines.WriteString(mustJSON(t, Event{
			V: 1, ID: "big" + strconv.Itoa(i), TS: "2026-05-10T00:00:00.000Z", Kind: "dump",
			Ctx: Context{Type: "cli", Site: "x"}, Data: json.RawMessage(strconv.Quote(big)),
		}))
	}
	dialAndSend(t, s.Addr(), lines.String())
	// Only once the writer holds an event has the server certainly taken the
	// connection; before that, no open connection can also mean not yet accepted.
	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("the writer never took an event")
	}
	if !waitFor(func() bool { return s.openConns() == 0 }, 2*time.Second) {
		t.Fatal("the connection waited")
	}
	release()
	waitFor(func() bool { return s.Len() > 0 }, 2*time.Second)
	time.Sleep(100 * time.Millisecond)
	if n := s.Len(); n == 0 || n > 3 {
		t.Errorf("kept %d events of ~4 KiB under a 10 KiB budget", n)
	}
}

// holdWriter makes the writer wait before each store until release is called,
// and says on reached when it first does, so a test can act while events are
// known to be queued.
func holdWriter(t *testing.T) (reached <-chan struct{}, release func()) {
	gate, at := make(chan struct{}), make(chan struct{})
	var first sync.Once
	prev := beforeStore
	beforeStore = func() {
		first.Do(func() { close(at) })
		<-gate
	}
	t.Cleanup(func() { beforeStore = prev })
	var once sync.Once
	return at, func() { once.Do(func() { close(gate) }) }
}

// queueBehindHeldWriter sends one event, which the held writer takes, and four
// more that stay in the queue behind it.
func queueBehindHeldWriter(t *testing.T, s *Server, reached <-chan struct{}, line func(int) string) {
	t.Helper()
	dialAndSend(t, s.Addr(), line(0))
	select {
	case <-reached:
	case <-time.After(2 * time.Second):
		t.Fatal("the writer never took the first event")
	}
	for i := 1; i < 5; i++ {
		dialAndSend(t, s.Addr(), line(i))
	}
	if !waitFor(func() bool { return s.openConns() == 0 && len(s.queue) == 4 }, 2*time.Second) {
		t.Fatalf("queued %d of 4 events", len(s.queue))
	}
}

// Clearing the buffer also clears what was still queued, so events from
// before a clear never come back after it.
func TestServer_ClearCoversQueuedEvents(t *testing.T) {
	reached, release := holdWriter(t)
	s := startServer(t)
	t.Cleanup(release)
	queueBehindHeldWriter(t, s, reached, func(i int) string { return floodEvent(t, i) })
	cleared := make(chan struct{})
	go func() { s.Clear(); close(cleared) }()
	settleBeforeRelease(cleared)
	release()
	<-cleared
	time.Sleep(100 * time.Millisecond)
	if n := s.Len(); n != 0 {
		t.Errorf("%d events queued before the clear came back after it", n)
	}
}

// Switching test recording off drops the test events still queued too.
func TestServer_TestRecordingOffCoversQueuedEvents(t *testing.T) {
	reached, release := holdWriter(t)
	s := startServer(t)
	t.Cleanup(release)
	s.SetKeepTests(true)
	queueBehindHeldWriter(t, s, reached, func(i int) string {
		return mustJSON(t, Event{
			V: 1, ID: "t" + strconv.Itoa(i), TS: "2026-05-10T00:00:00.000Z", Kind: "dump",
			Ctx: Context{Type: "cli", Site: "x", Test: true},
		})
	})
	off := make(chan struct{})
	go func() { s.SetKeepTests(false); close(off) }()
	settleBeforeRelease(off)
	release()
	<-off
	time.Sleep(100 * time.Millisecond)
	if n := s.Len(); n != 0 {
		t.Errorf("%d queued test events stored after recording was switched off", n)
	}
}

// settleBeforeRelease lets a removal run before the held writer is released: one
// that ignores the queue finishes at once, one that waits for it blocks.
func settleBeforeRelease(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}
}
