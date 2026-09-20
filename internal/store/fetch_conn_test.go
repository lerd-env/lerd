package store

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// A catalogue refresh fetches in parallel, so the idle pool has to be big enough
// to hold the whole wave. Go's default keeps two idle connections per host and
// closes the rest, which makes every wave after the first pay a fresh TCP and
// TLS handshake for each definition it pulls.
//
// What is measured is the second wave rather than the total, because the total
// cannot be pinned down: asked for a connection the transport races a fresh dial
// against waiting for an idle one and keeps whichever answers first, so the
// first wave opens seven or eight and either is fine. The second wave is the
// one that tells the story. Pooled it opens next to nothing; with Go's default
// of two idle it opens around six of its eight again.
func TestFetch_ReusesTheConnectionsAcrossParallelWaves(t *testing.T) {
	var conns atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	wave := func() {
		var wg sync.WaitGroup
		for range FetchConcurrency {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := c.fetch("x.yaml"); err != nil {
					t.Errorf("fetch: %v", err)
				}
			}()
		}
		wg.Wait()
	}
	wave()
	first := conns.Load()
	wave()
	second := conns.Load() - first

	// Half a wave is well clear of both: a pooled second wave opens none or one,
	// an unpooled one opens most of eight.
	if second >= FetchConcurrency/2 {
		t.Errorf("the second wave opened %d connections of its own after the first opened %d, want fewer than %d: it is handshaking again rather than reusing the pool",
			second, first, FetchConcurrency/2)
	}
}
