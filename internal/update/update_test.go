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

// Every redirect status GitHub may answer with has to be followed, not just
// 301/302: a 303/307/308 hop used to be rejected as an unexpected status.
func TestFetchLatestVersion_followsEveryRedirectStatus(t *testing.T) {
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound,
		http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", srv.URL+"/releases/tag/v2.0.0")
			w.WriteHeader(code)
		}))

		restore := stubURLs(&ReleaseBaseURLs, []string{srv.URL})
		got, err := FetchLatestVersion()
		restore()
		srv.Close()

		if err != nil {
			t.Errorf("HTTP %d: unexpected error: %v", code, err)
			continue
		}
		if got != "v2.0.0" {
			t.Errorf("HTTP %d: got %q, want v2.0.0", code, got)
		}
	}
}

// A chain that has already been over https must never be walked back down to
// plaintext: the next request would carry the update lookup in the clear and a
// network attacker could pick the tag we go on to download.
func TestFetchLatestVersion_refusesHTTPSDowngrade(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v6.6.6", http.StatusFound)
	}))
	defer plain.Close()

	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", plain.URL+"/latest")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer secure.Close()

	orig := redirectTransport
	redirectTransport = secure.Client().Transport
	defer func() { redirectTransport = orig }()
	defer stubURLs(&ReleaseBaseURLs, []string{secure.URL})()

	got, err := FetchLatestVersion()
	if err == nil {
		t.Fatalf("expected a refusal, got tag %q from a plaintext hop", got)
	}
	if !strings.Contains(err.Error(), "http") {
		t.Errorf("error should name the insecure hop, got: %v", err)
	}
}

// The tag becomes part of a filename joined onto a temp dir, so anything that
// can carry a path out of it has to be rejected at the source.
func TestFetchLatestVersion_rejectsUnsafeTag(t *testing.T) {
	for _, tag := range []string{"..%2f..%2fetc%2fpasswd", "sub/dir", "v1.0.0 rm -rf", "v1.0.0'"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/releases/tag/"+tag)
			w.WriteHeader(http.StatusFound)
		}))

		restore := stubURLs(&ReleaseBaseURLs, []string{srv.URL})
		got, err := FetchLatestVersion()
		restore()
		srv.Close()

		if err == nil {
			t.Errorf("tag %q was accepted as %q, want a refusal", tag, got)
		}
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
