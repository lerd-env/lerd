package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

func setupDebugbar(t *testing.T) *dumps.Server {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, s := range []config.Site{
		{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), Debugbar: true},
		{Name: "blog", Domains: []string{"blog.test"}, Path: t.TempDir()},
	} {
		if err := config.AddSite(s); err != nil {
			t.Fatal(err)
		}
	}
	srv := withDumpsServer(t)
	for _, e := range []dumps.Event{
		ev("1", "2026-10-04T10:00:00.900Z", dumps.KindBrowser, "page1", "shop", "browser", map[string]any{"type": "request", "message": "200 GET https://api.test/cart", "method": "GET", "request": "https://api.test/cart", "status": 200, "rid": "api9", "via": "fetch", "cross": true}),
		ev("2", "2026-10-04T10:00:01.000Z", dumps.KindRequest, "page1", "shop", "fpm", map[string]any{"method": "GET", "uri": "/cart", "status": 200}),
		ev("3", "2026-10-04T10:00:01.100Z", dumps.KindRequest, "api9", "api", "fpm", map[string]any{"method": "GET", "uri": "/cart", "status": 200}),
		ev("4", "2026-10-04T10:00:01.200Z", dumps.KindRequest, "blog1", "blog", "fpm", map[string]any{"method": "GET", "uri": "/", "status": 200}),
	} {
		srv.Push(e)
	}
	return srv
}

func barRequest(path, site string, viaNginx bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("X-Lerd-Site", site)
	if viaNginx {
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyUnixSocket{}, true))
	} else {
		r.RemoteAddr = "192.0.2.50:4000"
	}
	w := httptest.NewRecorder()
	withDebugbar(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

// A site reads its own requests and those its pages sent, never another
// site's, and a site without the bar reads nothing.
func TestDebugbar_RequestDetailIsScopedToTheSite(t *testing.T) {
	setupDebugbar(t)
	for _, c := range []struct {
		path, site string
		want       int
	}{
		{"/_lerd/bar/requests/page1", "shop", http.StatusOK},
		{"/_lerd/bar/requests/api9", "shop", http.StatusOK},
		{"/_lerd/bar/requests/blog1", "shop", http.StatusNotFound},
		{"/_lerd/bar/requests/blog1", "blog", http.StatusNotFound},
		{"/_lerd/bar/requests/nope", "shop", http.StatusNotFound},
	} {
		if w := barRequest(c.path, c.site, true); w.Code != c.want {
			t.Errorf("%s as %s: status %d, want %d", c.path, c.site, w.Code, c.want)
		}
	}
}

func TestDebugbar_RefusesAClientThatBypassesNginx(t *testing.T) {
	setupDebugbar(t)
	if w := barRequest("/_lerd/bar/requests/page1", "shop", false); w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
}

func TestDebugbar_ScriptCarriesTheSettings(t *testing.T) {
	setupDebugbar(t)
	cfg, _ := config.LoadGlobal()
	cfg.Debugbar.Style = "compact"
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	orig := debugbarBundle
	debugbarBundle = func() ([]byte, error) { return []byte("/* bar */"), nil }
	t.Cleanup(func() { debugbarBundle = orig })
	w := barRequest("/_lerd/bar/bar.js", "shop", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{`"style":"compact"`, `"corner":"bottom-right"`, `"base":"/_lerd/browser/bar/"`, `"themes":`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in the script's config", want)
		}
	}
	if !strings.HasPrefix(body, "(function(__lerdBarConfig){") {
		t.Errorf("script not wrapped: %.60s", body)
	}
}

func TestDebugbarSettings_RefusesAnUnknownValue(t *testing.T) {
	setupDebugbar(t)
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/debugbar/settings", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:4000"
		w := httptest.NewRecorder()
		handleDebugbarSettings(w, r)
		return w
	}
	if w := post(`{"corner":"middle"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "middle") {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if w := post(`{"style":"compact","edge":"top"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"edge":"top"`) {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	cfg, _ := config.LoadGlobal()
	if cfg.Debugbar.Style != "compact" {
		t.Fatalf("saved %+v", cfg.Debugbar)
	}
}

func barLocalRequest(path string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("X-Lerd-Site", "shop")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range headers {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyUnixSocket{}, true))
	w := httptest.NewRecorder()
	withDebugbar(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

// The site's code is read through the bar only for this machine's own page:
// never with the LAN exposed, through a tunnel, or for another page's fetch.
func TestDebugbar_SourceOnlyForTheLocalPage(t *testing.T) {
	setupDebugbar(t)
	site, _ := config.FindSite("shop")
	file := site.Path + "/app.php"
	if err := os.WriteFile(file, []byte("<?php\necho 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "/_lerd/bar/source?line=2&file=" + url.QueryEscape(file)
	if w := barLocalRequest(path, nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "echo 1;") {
		t.Fatalf("local page: status %d: %s", w.Code, w.Body)
	}
	for name, h := range map[string]map[string]string{
		"tunnel":       {"X-Forwarded-For": "203.0.113.9"},
		"cross-site":   {"Sec-Fetch-Site": "cross-site"},
		"no sec-fetch": {"Sec-Fetch-Site": ""},
	} {
		if w := barLocalRequest(path, h); w.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, w.Code)
		}
	}
	cfg, _ := config.LoadGlobal()
	cfg.LAN.Exposed = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	if w := barLocalRequest(path, nil); w.Code != http.StatusForbidden {
		t.Errorf("LAN exposed: status %d, want 403", w.Code)
	}
}
