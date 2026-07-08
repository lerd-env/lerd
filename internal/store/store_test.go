package store

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()

	index := Index{
		Frameworks: []IndexEntry{
			{
				Name:     "laravel",
				Label:    "Laravel",
				Versions: []string{"11", "10"},
				Latest:   "11",
				Detect: []config.FrameworkRule{
					{File: "artisan"},
					{Composer: "laravel/framework"},
				},
			},
			{
				Name:     "symfony",
				Label:    "Symfony",
				Versions: []string{"7", "6"},
				Latest:   "7",
				Detect: []config.FrameworkRule{
					{Composer: "symfony/framework-bundle"},
				},
			},
		},
	}

	laravelYAML := `name: laravel
label: Laravel
version: "11"
public_dir: public
detect:
  - file: artisan
  - composer: laravel/framework
console: artisan
`

	symfonyYAML := `name: symfony
label: Symfony
version: "7"
public_dir: public
detect:
  - composer: symfony/framework-bundle
console: bin/console
`

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(index)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data) //nolint:errcheck
	})
	mux.HandleFunc("/laravel/11.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(laravelYAML)) //nolint:errcheck
	})
	mux.HandleFunc("/symfony/7.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(symfonyYAML)) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	return httptest.NewServer(mux)
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return &Client{
		BaseURL: srv.URL,
	}
}

// When the primary base returns a non-200, the client must transparently fetch
// from the fallback base. This is the geodro->lerd-env transition path.
func TestFetchIndex_FallsBackWhenPrimaryFails(t *testing.T) {
	good := testServer(t)
	defer good.Close()

	c := &Client{
		BaseURL:   good.URL + "/missing", // /missing/index.json -> 404
		Fallbacks: []string{good.URL},
	}
	idx, err := c.FetchIndex()
	if err != nil {
		t.Fatalf("FetchIndex should succeed via fallback: %v", err)
	}
	if len(idx.Frameworks) == 0 || idx.Frameworks[0].Name != "laravel" {
		t.Fatalf("unexpected index from fallback: %+v", idx)
	}
}

// When every base fails (e.g. an internet outage), the client returns an error
// rather than silently changing anything.
func TestFetchIndex_AllBasesFail(t *testing.T) {
	good := testServer(t)
	dead := good.URL
	good.Close() // now refuses connections

	c := &Client{
		BaseURL:   dead,
		Fallbacks: []string{dead + "/also-dead"},
	}
	if _, err := c.FetchIndex(); err == nil {
		t.Fatal("expected an error when all bases are unreachable")
	}
}

// NewClient wires the framework-store URL from origin: the store content lives on
// lerd-env and is served directly, with no geodro fallback.
func TestNewClient_UsesNewOrgDirectly(t *testing.T) {
	c := NewClient()
	if !strings.Contains(c.BaseURL, "lerd-env") {
		t.Errorf("primary store URL = %q, want lerd-env", c.BaseURL)
	}
	if strings.Contains(c.BaseURL, "geodro") {
		t.Errorf("store URL must not rely on geodro, got %q", c.BaseURL)
	}
}

// NewServiceClient points at the dedicated lerd-env/services store.
func TestNewServiceClient_UsesServicesRepo(t *testing.T) {
	c := NewServiceClient()
	if !strings.Contains(c.BaseURL, "lerd-env/services") {
		t.Errorf("primary service-store URL = %q, want lerd-env/services", c.BaseURL)
	}
}

// ── FetchIndex ───────────────────────────────────────────────────────────────

func TestFetchIndex(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	idx, err := c.FetchIndex()
	if err != nil {
		t.Fatalf("FetchIndex() error: %v", err)
	}
	if len(idx.Frameworks) != 2 {
		t.Fatalf("expected 2 frameworks, got %d", len(idx.Frameworks))
	}
	if idx.Frameworks[0].Name != "laravel" {
		t.Errorf("expected first framework to be laravel, got %q", idx.Frameworks[0].Name)
	}
}

// ── FetchFramework ───────────────────────────────────────────────────────────

func TestFetchFramework(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	fw, err := c.FetchFramework("laravel", "11")
	if err != nil {
		t.Fatalf("FetchFramework() error: %v", err)
	}
	if fw.Name != "laravel" {
		t.Errorf("expected name=laravel, got %q", fw.Name)
	}
	if fw.Version != "11" {
		t.Errorf("expected version=11, got %q", fw.Version)
	}
	if fw.Console != "artisan" {
		t.Errorf("expected console=artisan, got %q", fw.Console)
	}
}

func TestFetchFramework_ResolvesLatest(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	fw, err := c.FetchFramework("laravel", "")
	if err != nil {
		t.Fatalf("FetchFramework() error: %v", err)
	}
	if fw.Version != "11" {
		t.Errorf("expected latest version=11, got %q", fw.Version)
	}
}

func TestFetchFramework_NotFound(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	_, err := c.FetchFramework("nonexistent", "1")
	if err == nil {
		t.Fatal("expected error for nonexistent framework")
	}
}

func TestFetchFramework_AlwaysFresh(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	// Fetch should always hit the server (no local cache).
	fw, err := c.FetchFramework("symfony", "7")
	if err != nil {
		t.Fatalf("FetchFramework() error: %v", err)
	}
	if fw.Name != "symfony" {
		t.Errorf("expected name=symfony, got %q", fw.Name)
	}

	// Stop server — second call should fail (no cache fallback).
	srv.Close()
	_, err = c.FetchFramework("symfony", "7")
	if err == nil {
		t.Error("expected error when server is down, got nil")
	}
}

// ── Search ───────────────────────────────────────────────────────────────────

func TestSearch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	results, err := c.Search("sym")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "symfony" {
		t.Errorf("expected symfony, got %q", results[0].Name)
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	results, err := c.Search("LARAVEL")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "laravel" {
		t.Errorf("expected laravel from case-insensitive search, got %v", results)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	results, err := c.Search("django")
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// ── DetectFromStore ──────────────────────────────────────────────────────────

func TestDetectFromStore_FileRule(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	dir := t.TempDir()
	// Create artisan file to trigger Laravel detection
	os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php"), 0o644) //nolint:errcheck

	entry, version, ok := c.DetectFromStore(dir)
	if !ok {
		t.Fatal("expected detection to succeed")
	}
	if entry.Name != "laravel" {
		t.Errorf("expected laravel, got %q", entry.Name)
	}
	if version != "11" {
		t.Errorf("expected version=11 (latest), got %q", version)
	}
}

func TestDetectFromStore_NoMatch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	dir := t.TempDir()
	_, _, ok := c.DetectFromStore(dir)
	if ok {
		t.Fatal("expected no detection in empty dir")
	}
}

func TestFetchWithRetry_RecoversFromTransientFailure(t *testing.T) {
	prevSleep := sleepFn
	sleepFn = func(time.Duration) {}
	t.Cleanup(func() { sleepFn = prevSleep })

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			http.Error(w, "boom", http.StatusBadGateway) // 502, transient
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	body, err := fetchWithRetry(&http.Client{Timeout: httpTimeout}, srv.URL)
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestFetchWithRetry_DoesNotRetry404(t *testing.T) {
	prevSleep := sleepFn
	sleepFn = func(time.Duration) {}
	t.Cleanup(func() { sleepFn = prevSleep })

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchWithRetry(&http.Client{Timeout: httpTimeout}, srv.URL); err == nil {
		t.Fatal("expected error for 404")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("a 4xx must not be retried, got %d attempts", got)
	}
}

func TestFetchWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	prevSleep := sleepFn
	sleepFn = func(time.Duration) {}
	t.Cleanup(func() { sleepFn = prevSleep })

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if _, err := fetchWithRetry(&http.Client{Timeout: httpTimeout}, srv.URL); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := atomic.LoadInt32(&calls); got != maxFetchAttempts {
		t.Fatalf("expected %d attempts, got %d", maxFetchAttempts, got)
	}
}
