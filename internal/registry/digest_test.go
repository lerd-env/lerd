package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManifestDigest_GHCRTokenFlow(t *testing.T) {
	var manifestHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			w.Write([]byte(`{"token":"stub"}`)) //nolint:errcheck
		case strings.HasSuffix(r.URL.Path, "/manifests/abc123"):
			manifestHits++
			if r.Method != http.MethodHead {
				t.Errorf("expected HEAD, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer stub" {
				t.Errorf("expected bearer token, got %q", got)
			}
			if !strings.Contains(r.Header.Get("Accept"), "image.index.v1+json") {
				t.Errorf("Accept header missing the index media type: %q", r.Header.Get("Accept"))
			}
			w.Header().Set("Docker-Content-Digest", "sha256:aaa")
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	got, err := ManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:abc123")
	if err != nil {
		t.Fatalf("ManifestDigest: %v", err)
	}
	if got != "sha256:aaa" {
		t.Errorf("digest = %q, want sha256:aaa", got)
	}

	// A second call is served from the on-disk cache, so the registry is hit once.
	if _, err := ManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:abc123"); err != nil {
		t.Fatalf("second ManifestDigest: %v", err)
	}
	if manifestHits != 1 {
		t.Errorf("registry hit %d times, want 1 (cache miss)", manifestHits)
	}

	// Dropping the cache entry forces a fresh fetch.
	if err := InvalidateManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:abc123"); err != nil {
		t.Fatalf("InvalidateManifestDigest: %v", err)
	}
	if _, err := ManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:abc123"); err != nil {
		t.Fatalf("third ManifestDigest: %v", err)
	}
	if manifestHits != 2 {
		t.Errorf("registry hit %d times after invalidate, want 2", manifestHits)
	}
}

// A tag that no longer exists must not be reported as a digest change: the
// caller would otherwise flag every image whose base was deleted as stale.
func TestManifestDigest_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"token":"stub"}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	if _, err := ManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:gone"); err == nil {
		t.Fatal("expected an error for a missing tag")
	}
}

// A registry that answers without the digest header is a fetch failure, not an
// empty digest that would later compare unequal to the recorded one.
func TestManifestDigest_MissingHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"token":"stub"}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	if _, err := ManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:abc123"); err == nil {
		t.Fatal("expected an error when the registry omits Docker-Content-Digest")
	}
}

// A failed fetch is cached as an empty entry so an unreachable registry is
// retried once per TTL rather than on every status rebuild.
func TestManifestDigest_FailureCachedAsMiss(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"token":"stub"}`)) //nolint:errcheck
			return
		}
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	for range 3 {
		if _, err := ManifestDigest("ghcr.io/lerd-env/lerd-php84-fpm-base:abc123"); err == nil {
			t.Fatal("expected an error from a 500")
		}
	}
	if hits != 1 {
		t.Errorf("registry hit %d times, want 1 (the failure is cached)", hits)
	}
}

func TestManifestDigest_AnonymousRegistry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Docker-Content-Digest", "sha256:bbb")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	got, err := ManifestDigest("registry.example.com/lerd/base:tag")
	if err != nil {
		t.Fatalf("ManifestDigest: %v", err)
	}
	if got != "sha256:bbb" {
		t.Errorf("digest = %q, want sha256:bbb", got)
	}
}

// dockerHubRegistry answers like Docker Hub: an anonymous token, then a
// manifest HEAD that needs it. down makes the HEAD fail.
func dockerHubRegistry(t *testing.T, digest string, down *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			if r.URL.Query().Get("service") != "registry.docker.io" {
				t.Errorf("token asked for service %q", r.URL.Query().Get("service"))
			}
			w.Write([]byte(`{"token":"anon"}`)) //nolint:errcheck
		case r.URL.Path == "/v2/library/nginx/manifests/alpine":
			if r.Method != http.MethodHead || r.Header.Get("Authorization") != "Bearer anon" {
				t.Errorf("want an authorised HEAD, got %s with %q", r.Method, r.Header.Get("Authorization"))
			}
			if down != nil && *down {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Docker-Content-Digest", digest)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// The digest comes from the registry podman pulls from, so it is the index
// digest podman records, never one platform's manifest.
func TestManifestDigest_DockerHubTokenFlow(t *testing.T) {
	srv := dockerHubRegistry(t, "sha256:index", nil)
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	got, err := ManifestDigest("docker.io/library/nginx:alpine")
	if err != nil || got != "sha256:index" {
		t.Errorf("ManifestDigest = %q, %v, want sha256:index", got, err)
	}
}

// A manual check that cannot reach the registry must not erase the digest a
// previous fetch learned: the update it showed is still pending.
func TestRefreshManifestDigest_FailureKeepsTheKnownDigest(t *testing.T) {
	down := false
	srv := dockerHubRegistry(t, "sha256:known", &down)
	defer srv.Close()
	withStubHTTP(t, srv)
	withTempCacheDir(t)

	if _, err := ManifestDigest("docker.io/library/nginx:alpine"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	down = true
	if _, err := RefreshManifestDigest("docker.io/library/nginx:alpine"); err == nil {
		t.Fatal("a refused refresh must still report its error")
	}
	if got, ok := CachedManifestDigest("docker.io/library/nginx:alpine"); !ok || got != "sha256:known" {
		t.Errorf("cached = %q, %v after a failed refresh, want sha256:known kept", got, ok)
	}
}
