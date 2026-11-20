package goserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// injectFixture is a PHP site with browser logs and the hello page plugin on,
// whose PHP answers with whatever page the test hands it, and lerd-ui played
// by a server that reports what reached it.
func injectFixture(t *testing.T, contentType, encoding, page string) (*Handler, *string) {
	t.Helper()
	h, _ := fixture(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadGlobal()
	cfg.SetDumpsEnabled(true)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	on := true
	site := h.Sites()[0]
	site.BrowserLogs = &config.BrowserLogs{Enabled: &on}
	site.PagePlugins = []string{"hello"}
	h.Sites = func() []config.Site { return []config.Site{site} }

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Lerd-Rid", "0123abcd0123abcd")
		if encoding != "" {
			w.Header().Set("Content-Encoding", encoding)
		}
		fmt.Fprint(w, page)
	}))
	h.Upstream = func(config.Site) string { return ln.Addr().String() }

	seen := new(string)
	ui := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*seen = fmt.Sprintf("%s %s site=%s branch=%s host=%s body=%s", r.Method, r.URL.Path,
			r.Header.Get("X-Lerd-Site"), r.Header.Get("X-Lerd-Branch"), r.Header.Get("X-Lerd-Host"), body)
		fmt.Fprint(w, "ui")
	}))
	t.Cleanup(ui.Close)
	h.WakeHold, _ = url.Parse(ui.URL)
	return h, seen
}

const wantTags = `<script src="/_lerd/browser.js" data-rid="0123abcd0123abcd"></script><script src="/_lerd/plugins/hello/page.js" defer></script></head>`

func TestInject_htmlPageGetsTheScriptsOnce(t *testing.T) {
	h, _ := injectFixture(t, "text/html; charset=utf-8", "", "<html><head><title>x</title></head><body></head></body></html>")
	rec := get(h, "GET", "http://app.test/", "")
	body := rec.Body.String()
	if strings.Count(body, wantTags) != 1 || !strings.HasPrefix(body, "<html><head><title>x</title>"+wantTags) {
		t.Fatalf("body %q", body)
	}
	if rec.Header().Get("Access-Control-Expose-Headers") != "X-Lerd-Rid" {
		t.Errorf("request id not exposed: %v", rec.Header())
	}
}

// Only an HTML page is rewritten; JSON is the app's data and a compressed body
// cannot be rewritten without corrupting it.
func TestInject_leavesOtherResponsesAlone(t *testing.T) {
	for _, c := range []struct{ ctype, enc, page string }{
		{"application/json", "", `{"html":"</head>"}`},
		{"text/html", "gzip", "\x1f\x8b</head>"},
	} {
		h, _ := injectFixture(t, c.ctype, c.enc, c.page)
		if body := get(h, "GET", "http://app.test/", "").Body.String(); body != c.page {
			t.Errorf("%s %s rewritten to %q", c.ctype, c.enc, body)
		}
	}
}

func TestInject_reporterAndPluginEndpointsReachLerdUI(t *testing.T) {
	h, seen := injectFixture(t, "text/html", "", "<head></head>")
	for _, c := range []struct{ method, path, body, want string }{
		{"POST", "/_lerd/browser", `{"e":1}`, `POST /_lerd/browser site=app branch= host=app.test body={"e":1}`},
		{"GET", "/_lerd/browser.js", "", "GET /_lerd/browser.js site=app"},
		{"GET", "/_lerd/plugins/hello/page.js", "", "GET /_lerd/plugins/hello/page.js site=app"},
	} {
		*seen = ""
		get(h, c.method, "http://app.test"+c.path, c.body)
		if !strings.HasPrefix(*seen, c.want) {
			t.Errorf("%s %s: lerd-ui saw %q, want %q", c.method, c.path, *seen, c.want)
		}
	}
}

// With browser logs and page plugins off, nothing is added and the endpoints
// are the app's own paths.
func TestInject_offAddsNothing(t *testing.T) {
	h, seen := injectFixture(t, "text/html", "", "<head></head>")
	site := h.Sites()[0]
	site.BrowserLogs, site.PagePlugins = nil, nil
	h.Sites = func() []config.Site { return []config.Site{site} }
	if body := get(h, "GET", "http://app.test/", "").Body.String(); body != "<head></head>" {
		t.Errorf("body %q", body)
	}
	get(h, "POST", "http://app.test/_lerd/browser", "x")
	if *seen != "" {
		t.Errorf("reached lerd-ui while off: %q", *seen)
	}
}
