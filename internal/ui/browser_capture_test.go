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

func setupBrowserCapture(t *testing.T, globalOn bool) *dumps.Server {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.LoadGlobal()
	cfg.BrowserCapture.Enabled = globalOn
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	return withDumpsServer(t)
}

func browserRequest(method, path, body string, viaNginx bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Lerd-Site", "shop")
	r.Header.Set("X-Lerd-Host", "shop.test")
	if viaNginx {
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyUnixSocket{}, true))
	} else {
		r.RemoteAddr = "192.0.2.50:4000"
	}
	w := httptest.NewRecorder()
	withBrowserCapture(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

func TestBrowserCapture_ReportLandsInTheRing(t *testing.T) {
	srv := setupBrowserCapture(t, true)
	w := browserRequest("POST", browserReportPath, `[{"type":"error","message":"boom","url":"https://shop.test/"}]`, true)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	got := srv.Filter(dumps.FilterOpts{Site: "shop", Kind: dumps.KindBrowser})
	if len(got) != 1 || got[0].Ctx.Domain != "shop.test" {
		t.Fatalf("ring = %+v", got)
	}
}

func TestBrowserCapture_RefusesAClientThatBypassesNginx(t *testing.T) {
	srv := setupBrowserCapture(t, true)
	if w := browserRequest("POST", browserReportPath, `[{"type":"error","message":"forged"}]`, false); w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
	if srv.Len() != 0 {
		t.Fatal("a forged report reached the ring")
	}
}

func TestBrowserCapture_DropsReportsWhileOff(t *testing.T) {
	srv := setupBrowserCapture(t, false)
	browserRequest("POST", browserReportPath, `[{"type":"error","message":"boom"}]`, true)
	if srv.Len() != 0 {
		t.Fatal("report recorded while browser capture is off")
	}
}

func TestBrowserCapture_ServesTheSiteScript(t *testing.T) {
	setupBrowserCapture(t, true)
	w := browserRequest("GET", browserScriptPath, "", true)
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("status %d, type %q", w.Code, w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), `"console":["error"]`) {
		t.Fatalf("default settings missing:\n%s", w.Body)
	}
}

func TestBrowserCapture_LeavesOtherPathsToTheMux(t *testing.T) {
	setupBrowserCapture(t, true)
	if w := browserRequest("GET", "/api/status", "", false); w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want the wrapped handler's 404", w.Code)
	}
}
