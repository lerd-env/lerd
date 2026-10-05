package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/dumps"
)

// withDumpsServer swaps the package-level receiver for a fresh in-memory
// instance for the duration of the test. The handler functions read the
// pointer at call time so each test gets a clean ring + hub.
func withDumpsServer(t *testing.T) *dumps.Server {
	t.Helper()
	srv, err := dumps.Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("dumps.Listen: %v", err)
	}
	prev := dumpsServer.Load()
	dumpsServer.Store(srv)
	t.Cleanup(func() {
		_ = srv.Close()
		dumpsServer.Store(prev)
	})
	return srv
}

func TestHandleDumpsList_EmptyWhenNoServer(t *testing.T) {
	prev := dumpsServer.Load()
	dumpsServer.Store(nil)
	t.Cleanup(func() { dumpsServer.Store(prev) })

	req := httptest.NewRequest("GET", "/api/dumps", nil)
	rec := httptest.NewRecorder()
	handleDumpsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []dumps.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty list, got %v", got)
	}
}

func TestHandleDumpsList_ReturnsBuffered(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(dumps.Event{V: 1, ID: "a", Kind: "dump", Ctx: dumps.Context{Type: "fpm", Site: "acme"}})
	srv.Push(dumps.Event{V: 1, ID: "b", Kind: "dump", Ctx: dumps.Context{Type: "cli", Site: "acme"}})

	req := httptest.NewRequest("GET", "/api/dumps", nil)
	rec := httptest.NewRecorder()
	handleDumpsList(rec, req)

	var got []dumps.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" {
		t.Errorf("got %v", got)
	}
}

func TestHandleDumpsList_FiltersBySite(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(dumps.Event{V: 1, ID: "a", Kind: "dump", Ctx: dumps.Context{Type: "fpm", Site: "acme"}})
	srv.Push(dumps.Event{V: 1, ID: "b", Kind: "dump", Ctx: dumps.Context{Type: "fpm", Site: "other"}})

	req := httptest.NewRequest("GET", "/api/dumps?site=acme", nil)
	rec := httptest.NewRecorder()
	handleDumpsList(rec, req)

	var got []dumps.Event
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("got %v", got)
	}
}

func TestHandleDumpsList_FiltersByBranch(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(dumps.Event{V: 1, ID: "a", Kind: "dump", Ctx: dumps.Context{Type: "fpm", Site: "acme"}})
	srv.Push(dumps.Event{V: 1, ID: "b", Kind: "dump", Ctx: dumps.Context{Type: "fpm", Site: "acme", Branch: "feature-x"}})

	req := httptest.NewRequest("GET", "/api/dumps?branch=feature-x", nil)
	rec := httptest.NewRecorder()
	handleDumpsList(rec, req)

	var got []dumps.Event
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("got %v", got)
	}
}

func TestHandleDumpsList_FiltersByCtxAndSince(t *testing.T) {
	srv := withDumpsServer(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		srv.Push(dumps.Event{V: 1, ID: id, Kind: "dump", Ctx: dumps.Context{Type: "fpm"}})
	}
	srv.Push(dumps.Event{V: 1, ID: "x-cli", Kind: "dump", Ctx: dumps.Context{Type: "cli"}})

	req := httptest.NewRequest("GET", "/api/dumps?ctx=fpm&since=b", nil)
	rec := httptest.NewRecorder()
	handleDumpsList(rec, req)
	var got []dumps.Event
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 2 || got[0].ID != "c" || got[1].ID != "d" {
		t.Errorf("got %v", got)
	}
}

func TestHandleDumpsClear_Loopback(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(dumps.Event{V: 1, ID: "a", Kind: "dump"})
	if srv.Len() != 1 {
		t.Fatal("setup: ring should have 1")
	}
	req := httptest.NewRequest("POST", "/api/dumps/clear", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleDumpsClear(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if srv.Len() != 0 {
		t.Errorf("ring not cleared, len = %d", srv.Len())
	}
}

func TestHandleDumpsClear_OneKindKeepsTheRest(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(dumps.Event{V: 1, ID: "a", Kind: "dump"})
	srv.Push(dumps.Event{V: 1, ID: "b", Kind: dumps.KindBrowser})
	req := httptest.NewRequest("POST", "/api/dumps/clear?kind=browser", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	handleDumpsClear(httptest.NewRecorder(), req)
	if got := srv.Snapshot(); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("ring = %+v, want only the dump", got)
	}
}

func TestHandleDumpsClear_RejectsNonLoopback(t *testing.T) {
	withDumpsServer(t)
	req := httptest.NewRequest("POST", "/api/dumps/clear", nil)
	req.RemoteAddr = "192.168.1.50:42000"
	rec := httptest.NewRecorder()
	handleDumpsClear(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleDumpsStatus_NoServerStillReturnsConfig(t *testing.T) {
	prev := dumpsServer.Load()
	dumpsServer.Store(nil)
	t.Cleanup(func() { dumpsServer.Store(prev) })

	req := httptest.NewRequest("GET", "/api/dumps/status", nil)
	rec := httptest.NewRecorder()
	handleDumpsStatus(rec, req)

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["listening"] != false {
		t.Errorf("listening = %v, want false", got["listening"])
	}
	if got["addr"] == "" {
		t.Errorf("addr should default to DefaultAddr even with no server")
	}
}

// flusherRecorder wraps httptest.ResponseRecorder so handleDumpsStream sees
// http.Flusher. The handler runs on its own goroutine while the test polls
// the body for SSE bytes, so writes and reads must be serialized — without
// the mutex, go test -race trips on every poll loop iteration.
type flusherRecorder struct {
	*httptest.ResponseRecorder
	mu      sync.Mutex
	flushes int
}

func (f *flusherRecorder) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(p)
}

// WriteString shadows ResponseRecorder.WriteString so io.WriteString takes
// the same mutex as Write. Without this, callers using io.WriteString
// bypass the lock and race against bodyBytes / bodyString.
func (f *flusherRecorder) WriteString(s string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.WriteString(s)
}

func (f *flusherRecorder) bodyString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Body.String()
}

func (f *flusherRecorder) bodyBytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Return a copy so the caller can read it without holding the lock.
	b := f.ResponseRecorder.Body.Bytes()
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (f *flusherRecorder) Flush() {
	f.mu.Lock()
	f.flushes++
	f.mu.Unlock()
}

func TestHandleDumpsStream_ReplaysSnapshotThenExitsOnContextCancel(t *testing.T) {
	srv := withDumpsServer(t)
	srv.Push(dumps.Event{V: 1, ID: "old1", Kind: "dump"})
	srv.Push(dumps.Event{V: 1, ID: "old2", Kind: "dump"})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/dumps/stream", nil).WithContext(ctx)
	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleDumpsStream(rec, req)
	}()

	// Wait until the replay events are in the body, then cancel. Use the
	// mutex-protected accessors since the handler goroutine is still writing.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Count(rec.bodyBytes(), []byte("data:")) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}

	body := rec.bodyString()
	for _, id := range []string{"old1", "old2"} {
		if !strings.Contains(body, id) {
			t.Errorf("SSE body missing %q\n--- body ---\n%s", id, body)
		}
	}
	if !strings.Contains(body, "text/event-stream") && rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", rec.Header().Get("Content-Type"))
	}
}

func TestHandleDumpsStream_DeliversLiveEvent(t *testing.T) {
	srv := withDumpsServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/dumps/stream", nil).WithContext(ctx)
	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleDumpsStream(rec, req)
	}()

	// Give the handler a moment to subscribe.
	time.Sleep(50 * time.Millisecond)
	srv.Push(dumps.Event{V: 1, ID: "live1", Kind: "dump"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.bodyString(), "live1") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	body := rec.bodyString()
	if !strings.Contains(body, "live1") {
		t.Errorf("live event missing\n--- body ---\n%s", body)
	}
}

// A stream opens on the newest events only; the ring keeps more for the
// Requests lens, which reads it from the server.
func TestHandleDumpsStream_ReplaysOnlyTheNewestEvents(t *testing.T) {
	srv := withDumpsServer(t)
	for i := 0; i < streamReplayLimit+5; i++ {
		srv.Push(dumps.Event{V: 1, ID: fmt.Sprintf("e%d", i), Kind: "dump"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/dumps/stream", nil).WithContext(ctx)
	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleDumpsStream(rec, req)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && bytes.Count(rec.bodyBytes(), []byte("data:")) < streamReplayLimit {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	body := rec.bodyString()
	if n := strings.Count(body, "data:"); n != streamReplayLimit {
		t.Fatalf("replayed %d events, want %d", n, streamReplayLimit)
	}
	if strings.Contains(body, `"id":"e4"`) || !strings.Contains(body, fmt.Sprintf(`"id":"e%d"`, streamReplayLimit+4)) {
		t.Errorf("want the newest events replayed and the oldest left out")
	}
}
