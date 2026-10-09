package dashboard

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Serving is what "is lerd up?" resolves to for the callers that start it when
// the answer is no, so it has to follow the vhost and nothing else.
func TestServingFollowsTheVhostProbe(t *testing.T) {
	orig := probe
	t.Cleanup(func() { probe = orig })

	var asked string
	probe = func(base string) bool { asked = base; return true }
	if !Serving() {
		t.Error("Serving() = false when the vhost answers")
	}
	if asked != VhostURL {
		t.Errorf("probed %q, want %q", asked, VhostURL)
	}

	probe = func(string) bool { return false }
	if Serving() {
		t.Error("Serving() = true when the vhost is down")
	}
}

func TestURLPrefersVhostWhenItServes(t *testing.T) {
	orig := probe
	t.Cleanup(func() { probe = orig })
	probe = func(string) bool { return true }

	if got := URL(); got != VhostURL {
		t.Errorf("URL() = %q, want %q", got, VhostURL)
	}
}

// A stopped stack has no nginx, so the vhost is refused and the only page the
// user can still reach is lerd-ui's own port.
func TestURLFallsBackToDirectPortWhenVhostIsDown(t *testing.T) {
	orig := probe
	t.Cleanup(func() { probe = orig })
	probe = func(string) bool { return false }

	if got := URL(); got != DirectURL {
		t.Errorf("URL() = %q, want %q", got, DirectURL)
	}
}

func TestServingAsksForTheDashboardPage(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
	}))
	defer srv.Close()

	if !serving(srv.URL) {
		t.Fatal("serving() = false for a responding server")
	}
	if path != "/" {
		t.Errorf("probed %q, want /", path)
	}
}

func TestServingIsFalseForAnUnreachableHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	if serving(url) {
		t.Error("serving() = true for a closed server")
	}
}

func TestServingIsFalseWhenTheProxyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if serving(srv.URL) {
		t.Error("serving() = true for a 502")
	}
}

// Windows' resolver cannot resolve *.localhost (only browsers special-case it),
// so the probe must reach the vhost through loopback by itself.
func TestServing_ReachesLocalhostNamesWithoutResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if !serving("http://lerd.localhost:" + port) {
		t.Fatal("serving() = false for a *.localhost name that loopback answers")
	}
}
