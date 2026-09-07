package update

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
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

// A token only ever goes to the real GitHub API over https. A
// LERD_RELEASES_API_URL override points at a mirror or a test rig, and the
// user's credentials must not follow it there.
func TestTokenFor_onlyAuthenticatesTheRealGitHubAPI(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	tests := []struct {
		url  string
		want string
	}{
		{"https://api.github.com/repos/lerd-env/lerd/releases", "ghp_secret"},
		{"https://API.GitHub.com/repos/lerd-env/lerd/releases", "ghp_secret"},
		{"http://api.github.com/repos/lerd-env/lerd/releases", ""},
		{"https://api.github.com.evil.test/repos/lerd-env/lerd/releases", ""},
		{"https://mirror.example.test/repos/lerd-env/lerd/releases", ""},
	}
	for _, tt := range tests {
		if got := tokenFor(tt.url); got != tt.want {
			t.Errorf("tokenFor(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// GH_TOKEN stands in when GITHUB_TOKEN is unset, and blank-but-set counts as
// no token at all.
func TestTokenFor_fallsBackToGHToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "   ")
	t.Setenv("GH_TOKEN", "gho_secret")
	if got := tokenFor("https://api.github.com/repos/lerd-env/lerd/releases"); got != "gho_secret" {
		t.Errorf("tokenFor() = %q, want gho_secret", got)
	}
}

// The pre-release fetch must not leak the token to an overridden API base.
func TestFetchLatestPrerelease_sendsNoTokenToAnOverriddenAPI(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	t.Setenv("GH_TOKEN", "gho_secret")

	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.32.0-beta.1","prerelease":true}]`))
	}))
	defer srv.Close()
	defer stubURLs(&APIBaseURLs, []string{srv.URL})()

	got, err := FetchLatestPrerelease()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.32.0-beta.1" {
		t.Errorf("FetchLatestPrerelease() = %q, want v1.32.0-beta.1", got)
	}
	if auth != "" {
		t.Errorf("Authorization header sent to a non-GitHub host: %q", auth)
	}
}

// An exhausted quota reads as a bare HTTP 403 unless the headers are read
// (#1640): the message has to name the rate limit, say when it comes back, and
// point at the token that raises it.
func TestFetchLatestPrerelease_reportsAnExhaustedRateLimit(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	reset := time.Now().Add(46 * time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	defer stubURLs(&APIBaseURLs, []string{srv.URL})()

	_, err := FetchLatestPrerelease()
	if err == nil {
		t.Fatal("expected an error when the rate limit is exhausted")
	}
	for _, want := range []string{"rate limit exhausted", "46 min", "GITHUB_TOKEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// A 429 carrying the same headers is the secondary-limit form of the same
// failure and gets the same message.
func TestFetchLatestPrerelease_reportsATooManyRequestsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	defer stubURLs(&APIBaseURLs, []string{srv.URL})()

	_, err := FetchLatestPrerelease()
	if err == nil || !strings.Contains(err.Error(), "rate limit exhausted") {
		t.Fatalf("expected a rate-limit error, got: %v", err)
	}
}

// A 403 with quota left is not a rate-limit problem and keeps the plain status
// error, so a real permission failure is not misdiagnosed.
func TestFetchLatestPrerelease_keepsPlainStatusErrorWithQuotaLeft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "57")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	defer stubURLs(&APIBaseURLs, []string{srv.URL})()

	_, err := FetchLatestPrerelease()
	if err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
	if strings.Contains(err.Error(), "rate limit") {
		t.Errorf("403 with quota left should not read as a rate limit, got: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error should carry the status, got: %v", err)
	}
}
