package update

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchLatestVersion_followsRenameRedirectChain pins the fix for #1296:
// a repo/org rename makes GitHub answer the old /releases/latest with a 301
// to the new /releases/latest, and only that second URL redirects to the tag.
// The checker must follow the chain to completion instead of parsing hop one.
func TestFetchLatestVersion_followsRenameRedirectChain(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			w.Header().Set("Location", srv.URL+"/renamed/releases/latest")
			w.WriteHeader(http.StatusMovedPermanently)
		case "/renamed/releases/latest":
			w.Header().Set("Location", srv.URL+"/renamed/releases/tag/v1.31.0")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	defer stubURLs(&ReleaseBaseURLs, []string{srv.URL})()

	got, err := FetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.31.0" {
		t.Errorf("FetchLatestVersion() = %q, want v1.31.0", got)
	}
}

// TestFetchLatestVersion_capsRedirectChain ensures a redirect loop that never
// reaches a /tag/ URL errors out instead of following it forever.
func TestFetchLatestVersion_capsRedirectChain(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL+r.URL.Path+"/latest")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer srv.Close()

	defer stubURLs(&ReleaseBaseURLs, []string{srv.URL})()

	_, err := FetchLatestVersion()
	if err == nil {
		t.Fatal("expected an error for a redirect chain that never yields a tag")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("error should mention redirects, got: %v", err)
	}
}
