package mcp

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// A client that closes stdin while a call runs must end the session at once,
// not after the call returns, so the call's context is what EOF cancels.
func TestReadRequests_cancelsTheSessionAtEOF(t *testing.T) {
	cancelled := false
	lines, scanErr := readRequests(strings.NewReader("{\"id\":1}\n{\"id\":2}\n"), func() { cancelled = true })
	var got []string
	for l := range lines {
		got = append(got, string(l))
	}
	if strings.Join(got, "|") != `{"id":1}|{"id":2}` {
		t.Fatalf("lines = %q", got)
	}
	if !cancelled {
		t.Fatal("EOF did not cancel the session")
	}
	if *scanErr != nil {
		t.Fatalf("scan error = %v", *scanErr)
	}
}

// Requests the client sends while a call is still running must not stop the
// reader from seeing the session close, or the call is never cancelled.
func TestReadRequests_seesEOFBehindQueuedRequests(t *testing.T) {
	cancelled := make(chan struct{})
	lines, _ := readRequests(strings.NewReader("{\"id\":1}\n{\"id\":2}\n{\"id\":3}\n"), func() { close(cancelled) })
	<-lines // the call being dispatched; the rest wait unread
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("EOF stuck behind requests nobody has read yet")
	}
	n := 0
	for range lines {
		n++
	}
	if n != 2 {
		t.Fatalf("delivered %d queued requests, want 2", n)
	}
}

// Everything sent during a call waits in memory, so a client that keeps sending
// has its session ended past the cap instead of growing the queue for good.
func TestReadRequests_endsTheSessionWhenTooMuchQueuesUp(t *testing.T) {
	r, w := io.Pipe()
	t.Cleanup(func() { w.Close() })
	cancelled := make(chan struct{})
	var once sync.Once
	lines, _ := readRequests(r, func() { once.Do(func() { close(cancelled) }) })
	if _, err := w.Write([]byte("{\"id\":1}\n")); err != nil {
		t.Fatal(err)
	}
	<-lines // the call being dispatched; what follows waits unread
	go func() {
		for i := 0; i < maxQueued+1; i++ {
			if _, err := w.Write([]byte("{\"id\":1}\n")); err != nil {
				return
			}
		}
	}()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("a queue past the cap did not end the session")
	}
	w.Close()
	n := 0
	for range lines {
		n++
	}
	if n != 0 {
		t.Fatalf("served %d requests after the session ended", n)
	}
}
