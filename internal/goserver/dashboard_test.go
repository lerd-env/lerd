package goserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// The dashboard paths are the same list nginx's lerd.localhost vhost carries;
// this reads it off nginx's pinned render so the two cannot drift apart.
func TestDashboard_pathsMatchTheNginxVhost(t *testing.T) {
	data, err := os.ReadFile("../nginx/testdata/golden/base-config/conf.d/lerd.localhost.conf")
	if err != nil {
		t.Fatal(err)
	}
	var exact, prefix []string
	for _, m := range regexp.MustCompile(`location (=|\^~) (\S+) \{`).FindAllStringSubmatch(string(data), -1) {
		if m[1] == "=" {
			exact = append(exact, m[2])
		} else if !strings.HasPrefix(m[2], "/rustfs/") { // a service mount, read from the config
			prefix = append(prefix, m[2])
		}
	}
	slices.Sort(exact)
	slices.Sort(prefix)
	gotExact, gotPrefix := slices.Sorted(slices.Values(dashboardExact)), slices.Sorted(slices.Values(dashboardPrefixes))
	if !slices.Equal(exact, gotExact) || !slices.Equal(prefix, gotPrefix) {
		t.Errorf("nginx serves exact %v prefix %v, the Go server %v %v", exact, prefix, gotExact, gotPrefix)
	}
}

func dashboardFixture(t *testing.T) *Handler {
	t.Helper()
	h, _ := fixture(t)
	ui := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ui %s host=%s", r.URL.Path, r.Host)
	}))
	t.Cleanup(ui.Close)
	h.WakeHold, _ = url.Parse(ui.URL)
	return h
}

func TestDashboard_allowedPathsReachLerdUI(t *testing.T) {
	h := dashboardFixture(t)
	for _, p := range []string{"/", "/assets/app.js", "/_svc/mailpit/", "/sw.js"} {
		if got := get(h, "GET", "http://lerd.localhost"+p, "").Body.String(); got != "ui "+p+" host=lerd.localhost" {
			t.Errorf("%s: %q", p, got)
		}
	}
}

// Anything else on the dashboard host is dropped without an answer, as nginx's
// return 444 does, so the API is reached only through lerd-ui's own listener.
func TestDashboard_otherPathsDropTheConnection(t *testing.T) {
	h := dashboardFixture(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprint(conn, "GET /api/sites HTTP/1.1\r\nHost: lerd.localhost\r\n\r\n")
	buf := make([]byte, 64)
	if n, err := conn.Read(buf); err == nil {
		t.Fatalf("got an answer: %q", buf[:n])
	}
}

func TestDefault_unknownHostGetsTheErrorPage(t *testing.T) {
	h, _ := fixture(t)
	os.MkdirAll(config.ErrorPagesDir(), 0o755)
	os.WriteFile(filepath.Join(config.ErrorPagesDir(), "404.html"), []byte("<h1>No site here</h1>"), 0o644)
	rec := get(h, "GET", "http://nobody.test/", "")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "No site here") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
