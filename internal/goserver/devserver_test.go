package goserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// devFixture is a site whose Vite dev server, on the host, is an httptest
// server that knows only the paths in serves.
func devFixture(t *testing.T, serves map[string]string) (*Handler, int) {
	t.Helper()
	h, _ := fixture(t)
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := serves[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "%s host=%s", body, r.Host)
	}))
	t.Cleanup(dev.Close)
	_, portStr, _ := net.SplitHostPort(strings.TrimPrefix(dev.URL, "http://"))
	port, _ := strconv.Atoi(portStr)
	site := h.Sites()[0]
	site.DevServerPort = port
	if err := os.MkdirAll(filepath.Join(site.Path, "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.Sites = func() []config.Site { return []config.Site{site} }
	h.WorkerHost = func(config.Site, bool) string { return "127.0.0.1" }
	return h, port
}

func TestDevServer_prefixGoesToTheDevServer(t *testing.T) {
	h, _ := devFixture(t, map[string]string{"/@lerd-vite/src/app.js": "vite module"})
	rec := get(h, "GET", "http://app.test/@lerd-vite/src/app.js", "")
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "vite module host=app.test") {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

// Plugin middleware ahead of the tool's own base handling never sees the
// prefix stripped, so a 404 is retried once without it, as the vhost does.
func TestDevServer_404RetriedWithoutThePrefix(t *testing.T) {
	h, _ := devFixture(t, map[string]string{"/src/app.js": "unprefixed"})
	rec := get(h, "GET", "http://app.test/@lerd-vite/src/app.js", "")
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "unprefixed") {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

// A stopped dev server answers a readable 502 any origin may read, so the
// page's console names the problem instead of reporting a CORS failure.
func TestDevServer_downAnswersAReadable502(t *testing.T) {
	h, port := devFixture(t, nil)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	site := h.Sites()[0]
	site.DevServerPort = dead
	h.Sites = func() []config.Site { return []config.Site{site} }
	rec := get(h, "GET", "http://app.test/@lerd-vite/x.js", "")
	if rec.Code != 502 || rec.Header().Get("Access-Control-Allow-Origin") != "*" ||
		!strings.Contains(rec.Body.String(), "nothing is listening on this site's dev server port ("+strconv.Itoa(dead)+")") {
		t.Fatalf("%d %v %q (live port %d)", rec.Code, rec.Header(), rec.Body.String(), port)
	}
}

func TestDevServer_otherPathsStayWithTheApp(t *testing.T) {
	h, _ := devFixture(t, map[string]string{"/": "vite"})
	if body := get(h, "GET", "http://app.test/", "").Body.String(); strings.HasPrefix(body, "vite") {
		t.Fatalf("the app's root went to the dev server: %q", body)
	}
}
