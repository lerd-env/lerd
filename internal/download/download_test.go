package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestVerified_AcceptsAMatchingDigest(t *testing.T) {
	shorten(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload")
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "tool")
	if err := Verified(context.Background(), srv.URL, dest, 0o755, sha256Of("payload"), io.Discard); err != nil {
		t.Fatalf("Verified: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "payload" {
		t.Fatalf("dest = %q, %v", b, err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, %v", info.Mode().Perm(), err)
	}
}

// Bytes that arrive intact but are the wrong ones are not a transient failure,
// so the attempt must not be retried and nothing may reach dest.
func TestVerified_RejectsAMismatchedDigest(t *testing.T) {
	shorten(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, "tampered")
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "tool")
	err := Verified(context.Background(), srv.URL, dest, 0o755, sha256Of("payload"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want a checksum mismatch", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("a wrong payload was fetched %d times, want 1", n)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("the rejected payload was written to dest")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.part-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

// A download that never succeeds must leave the tool that is already installed
// exactly as it was, rather than replacing it with a truncated file.
func TestVerified_FailedDownloadLeavesTheExistingBinaryIntact(t *testing.T) {
	shorten(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "64")
		fmt.Fprint(w, "half")
		w.(http.Flusher).Flush()
		srvHijackClose(w)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "tool")
	if err := os.WriteFile(dest, []byte("the working binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Verified(context.Background(), srv.URL, dest, 0o755, "", io.Discard); err == nil {
		t.Fatal("expected the truncated download to fail")
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "the working binary" {
		t.Errorf("the previously working binary was destroyed: %q, %v", b, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.part-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

// srvHijackClose cuts the connection mid-body so the client sees a truncated
// read rather than a clean EOF.
func srvHijackClose(w http.ResponseWriter) {
	if hj, ok := w.(http.Hijacker); ok {
		if conn, _, err := hj.Hijack(); err == nil {
			conn.Close()
		}
	}
}
