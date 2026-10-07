package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

// setupBrowserLogs sets the debug switch and the shop site's own opt-in.
func setupBrowserLogs(t *testing.T, debug, siteOn bool) *dumps.Server {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), BrowserLogs: &config.BrowserLogs{Enabled: &siteOn, Console: []string{"error"}}}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadGlobal()
	cfg.SetDumpsEnabled(debug)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	return withDumpsServer(t)
}

func browserRequest(method, path, body string, viaNginx bool, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	r.Header.Set("X-Lerd-Site", "shop")
	r.Header.Set("X-Lerd-Host", "shop.test")
	if viaNginx {
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyUnixSocket{}, true))
	} else {
		r.RemoteAddr = "192.0.2.50:4000"
	}
	w := httptest.NewRecorder()
	withBrowserLogs(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

func TestBrowserLogs_ReportLandsInTheRing(t *testing.T) {
	srv := setupBrowserLogs(t, true, true)
	w := browserRequest("POST", browserReportPath, `[{"type":"error","message":"boom","url":"https://shop.test/"}]`, true)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	got := srv.Filter(dumps.FilterOpts{Site: "shop", Kind: dumps.KindBrowser})
	if len(got) != 1 || got[0].Ctx.Domain != "shop.test" {
		t.Fatalf("ring = %+v", got)
	}
}

func TestBrowserLogs_RefusesAClientThatBypassesNginx(t *testing.T) {
	srv := setupBrowserLogs(t, true, true)
	if w := browserRequest("POST", browserReportPath, `[{"type":"error","message":"forged"}]`, false); w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
	if srv.Len() != 0 {
		t.Fatal("a forged report reached the ring")
	}
}

// A page cached from before capture was turned off may still post.
func TestBrowserLogs_DropsReportsWhileOff(t *testing.T) {
	for _, tc := range []struct {
		name          string
		debug, siteOn bool
	}{
		{"debug off", false, true},
		{"site not opted in", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupBrowserLogs(t, tc.debug, tc.siteOn)
			browserRequest("POST", browserReportPath, `[{"type":"error","message":"boom"}]`, true)
			if srv.Len() != 0 {
				t.Fatal("report recorded while browser logs are off")
			}
		})
	}
}

func TestBrowserLogs_ServesTheSiteScript(t *testing.T) {
	setupBrowserLogs(t, true, true)
	w := browserRequest("GET", browserScriptPath, "", true)
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("status %d, type %q", w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), `"console":["error"]`) {
		t.Fatalf("default settings missing:\n%s", w.Body)
	}
}

func TestBrowserLogs_LeavesOtherPathsToTheMux(t *testing.T) {
	setupBrowserLogs(t, true, true)
	if w := browserRequest("GET", "/api/status", "", false); w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want the wrapped handler's 404", w.Code)
	}
}

// A page on another site can fire a simple POST at the endpoint; the browser's
// own provenance headers, which a page cannot set, are what turn it away.
func TestBrowserLogs_RefusesAReportFromAnotherSite(t *testing.T) {
	report := `[{"type":"error","message":"planted"}]`
	for _, tc := range []struct {
		name    string
		headers []string
		want    int
	}{
		{"same-origin fetch", []string{"Sec-Fetch-Site", "same-origin", "Origin", "https://shop.test"}, http.StatusNoContent},
		{"cross-site page", []string{"Sec-Fetch-Site", "cross-site", "Origin", "https://evil.example"}, http.StatusForbidden},
		{"same-site subdomain", []string{"Sec-Fetch-Site", "same-site", "Origin", "https://other.shop.test"}, http.StatusForbidden},
		{"older browser, other origin", []string{"Origin", "https://evil.example"}, http.StatusForbidden},
		{"older browser, own origin", []string{"Origin", "https://shop.test"}, http.StatusNoContent},
		{"no browser headers", nil, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupBrowserLogs(t, true, true)
			w := browserRequest("POST", browserReportPath, report, true, tc.headers...)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if landed := srv.Len() > 0; landed != (tc.want == http.StatusNoContent) {
				t.Fatalf("report landed = %v with status %d", landed, w.Code)
			}
		})
	}
}
