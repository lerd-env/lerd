package download

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shorten makes retries and the stall watchdog fast for tests, restoring the
// real values on cleanup.
func shorten(t *testing.T) {
	t.Helper()
	origDelay, origIdle := retryDelay, idleTimeout
	retryDelay = 10 * time.Millisecond
	idleTimeout = 200 * time.Millisecond
	t.Cleanup(func() { retryDelay, idleTimeout = origDelay, origIdle })
}

func TestFile_WritesDestWithMode(t *testing.T) {
	shorten(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	var out bytes.Buffer
	if err := File(context.Background(), srv.URL, dest, 0o755, &out); err != nil {
		t.Fatalf("File: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("dest contents = %q", got)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("dest mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestFile_RetriesTransient5xx(t *testing.T) {
	shorten(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		fmt.Fprint(w, "ok after retries")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	if err := File(context.Background(), srv.URL, dest, 0o644, &bytes.Buffer{}); err != nil {
		t.Fatalf("File: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("server calls = %d, want 3", calls.Load())
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "ok after retries" {
		t.Fatalf("dest contents = %q", got)
	}
}

func TestFile_GivesUpAfterAttempts(t *testing.T) {
	shorten(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	err := File(context.Background(), srv.URL, dest, 0o644, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "HTTP 503 for "+srv.URL) {
		t.Fatalf("error = %v, want the plain HTTP status error", err)
	}
	if calls.Load() != int32(attempts) {
		t.Fatalf("server calls = %d, want %d", calls.Load(), attempts)
	}
}

func TestFile_FailsFastOnClientError(t *testing.T) {
	shorten(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	err := File(context.Background(), srv.URL, dest, 0o644, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error = %v, want HTTP 404", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("server calls = %d, want 1 (no retry on 404)", calls.Load())
	}
}

func TestFile_RetriesTruncatedBody(t *testing.T) {
	shorten(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			// Claim more bytes than we send so the copy fails mid-body.
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "partial")
			return
		}
		fmt.Fprint(w, "complete body")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	if err := File(context.Background(), srv.URL, dest, 0o644, &bytes.Buffer{}); err != nil {
		t.Fatalf("File: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "complete body" {
		t.Fatalf("dest contents = %q, want the retried full body", got)
	}
}

func TestFile_StalledTransferRetries(t *testing.T) {
	shorten(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "stall")
			w.(http.Flusher).Flush()
			<-r.Context().Done() // hang until the watchdog cancels the attempt
			return
		}
		fmt.Fprint(w, "fresh")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	done := make(chan error, 1)
	go func() {
		done <- File(context.Background(), srv.URL, dest, 0o644, &bytes.Buffer{})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("File: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stalled transfer was never cancelled")
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "fresh" {
		t.Fatalf("dest contents = %q", got)
	}
}

func TestFile_CancelledContextStopsRetrying(t *testing.T) {
	shorten(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := filepath.Join(t.TempDir(), "bin")
	if err := File(ctx, srv.URL, dest, 0o644, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error with a cancelled context")
	}
	if calls.Load() > 1 {
		t.Fatalf("server calls = %d, want at most 1 after cancellation", calls.Load())
	}
}
