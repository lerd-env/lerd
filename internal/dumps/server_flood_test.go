package dumps

import (
	"strconv"
	"strings"
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
