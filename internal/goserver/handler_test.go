package goserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fixture is one PHP site served through a stand-in PHP-FPM that reports which
// script it was asked to run, with which URI and body.
func fixture(t *testing.T) (*Handler, string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	pub := filepath.Join(root, "public")
	for name, body := range map[string]string{"index.php": "", "info.php": "", "app.css": "body{}", ".env": "SECRET=1"} {
		if err := os.MkdirAll(pub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pub, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := fcgi.ProcessEnv(r)
		body, _ := io.ReadAll(r.Body)
		// Go's responder consumes SCRIPT_NAME and REQUEST_URI into the request,
		// so the script comes from DOCUMENT_URI, which carries the same value.
		fmt.Fprintf(w, "%s|%s|%s|%s|%s", env["SCRIPT_FILENAME"], env["DOCUMENT_URI"], r.URL.RequestURI(), env["DOCUMENT_ROOT"], body)
	}))
	site := config.Site{Name: "app", Domains: []string{"app.test"}, Path: root, PHPVersion: "8.4"}
	h := &Handler{
		Sites:    func() []config.Site { return []config.Site{site} },
		Upstream: func(config.Site) string { return ln.Addr().String() },
	}
	return h, pub
}

func get(h http.Handler, method, url, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
	return rec
}

func TestHandler_routesLikeTheNginxVhost(t *testing.T) {
	h, pub := fixture(t)
	cases := []struct {
		name, method, url, body string
		status                  int
		want                    string
	}{
		{"root runs the front controller", "GET", "http://app.test/", "", 200, pub + "/index.php|/index.php|/|" + pub + "|"},
		{"unknown path runs the front controller", "GET", "http://app.test/users/7?tab=a", "", 200, pub + "/index.php|/index.php|/users/7?tab=a|" + pub + "|"},
		{"an existing php file runs itself", "GET", "http://app.test/info.php", "", 200, pub + "/info.php|/info.php|/info.php|"},
		{"post body reaches php", "POST", "http://app.test/save", "hello", 200, "|hello"},
		{"static file served directly", "GET", "http://app.test/app.css", "", 200, "body{}"},
		{"subdomain of the site is the site", "GET", "http://www.app.test/", "", 200, pub + "/index.php|"},
		{"dotfile never served", "GET", "http://app.test/.env", "", 403, ""},
		{"unknown host", "GET", "http://other.test/", "", 404, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := get(h, c.method, c.url, c.body)
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d; body %q", rec.Code, c.status, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), c.want) {
				t.Errorf("body %q, want it to contain %q", rec.Body.String(), c.want)
			}
		})
	}
}

// A PHP-FPM that is down answers 502 rather than hanging the browser.
func TestHandler_deadUpstreamIs502(t *testing.T) {
	h, _ := fixture(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := ln.Addr().String()
	ln.Close()
	h.Upstream = func(config.Site) string { return dead }
	if rec := get(h, "GET", "http://app.test/", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
}
