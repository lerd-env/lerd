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
	wave()

	if n := conns.Load(); n > FetchConcurrency {
		t.Errorf("opened %d connections for two waves of %d fetches, want at most %d",
			n, FetchConcurrency, FetchConcurrency)
	}
}
