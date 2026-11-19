package goserver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// serviceFixture serves mail.lerd.test from an upstream that answers with CORS
// headers of its own, which lerd has to replace rather than add to.
func serviceFixture(t *testing.T, cors bool) *Handler {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("ETag", `"abc"`)
		fmt.Fprintf(w, "host=%s xfp=%s method=%s", r.Host, r.Header.Get("X-Forwarded-Proto"), r.Method)
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	return &Handler{
		Sites:    func() []config.Site { return nil },
		Upstream: func(config.Site) string { return "" },
		Services: func() []ServiceRoute {
			return []ServiceRoute{{Domain: "mail.lerd.test", Upstream: u.Host, CORS: cors, TLD: "test"}}
		},
	}
}

func serviceReq(h *Handler, method, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://mail.lerd.test/inbox", nil)
	req.TLS = &tlsState
	if origin != "" {
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Headers", "x-amz-date, content-type")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestService_proxiedOverHTTPSKeepingTheHost(t *testing.T) {
	h := serviceFixture(t, false)
	rec := serviceReq(h, "GET", "")
	if body := rec.Body.String(); rec.Code != 200 || !strings.Contains(body, "host=mail.lerd.test") || !strings.Contains(body, "xfp=https") {
		t.Fatalf("%d %q", rec.Code, body)
	}
}

func TestService_plainHTTPMovesPermanentlyToHTTPS(t *testing.T) {
	h := serviceFixture(t, false)
	rec := get(h, "GET", "http://mail.lerd.test/inbox?a=1", "")
	if rec.Code != 301 || rec.Header().Get("Location") != "https://mail.lerd.test/inbox?a=1" {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestService_corsAbsentUnlessAskedFor(t *testing.T) {
	h := serviceFixture(t, false)
	rec := serviceReq(h, "GET", "https://app.test")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("without cors the upstream's own header stays, got %q", got)
	}
}

func TestService_corsPreflightAnsweredForALerdOrigin(t *testing.T) {
	h := serviceFixture(t, true)
	rec := serviceReq(h, "OPTIONS", "https://app.test")
	hd := rec.Header()
	if rec.Code != 204 || hd.Get("Access-Control-Allow-Origin") != "https://app.test" ||
		hd.Get("Access-Control-Allow-Headers") != "x-amz-date, content-type" ||
		hd.Get("Access-Control-Allow-Methods") != "GET, PUT, POST, DELETE, HEAD, OPTIONS" ||
		hd.Get("Access-Control-Max-Age") != "86400" || hd.Get("Vary") != "Origin" {
		t.Fatalf("%d %v", rec.Code, hd)
	}
	if strings.Contains(rec.Body.String(), "method=OPTIONS") {
		t.Error("the preflight reached the upstream")
	}
}

func TestService_corsReplacesTheUpstreamHeaderAndExposesETag(t *testing.T) {
	h := serviceFixture(t, true)
	rec := serviceReq(h, "GET", "https://app.test:8443")
	hd := rec.Header()
	if v := hd.Values("Access-Control-Allow-Origin"); len(v) != 1 || v[0] != "https://app.test:8443" {
		t.Errorf("Allow-Origin %v", v)
	}
	if hd.Get("Access-Control-Expose-Headers") != "ETag" || hd.Get("Vary") != "Origin" {
		t.Errorf("headers %v", hd)
	}
}

// A page on another origin gets no Allow-Origin at all, so the browser blocks
// it exactly as with no CORS configured.
func TestService_corsRefusesAForeignOrigin(t *testing.T) {
	h := serviceFixture(t, true)
	for _, origin := range []string{"https://evil.example", "https://app.test.evil.example", "http://test"} {
		if got := serviceReq(h, "GET", origin).Header().Values("Access-Control-Allow-Origin"); len(got) != 0 {
			t.Errorf("%s got Allow-Origin %v", origin, got)
		}
	}
}

func TestService_certificateFromItsOwnFile(t *testing.T) {
	h := serviceFixture(t, false)
	writeSiteCert(t, "mail.lerd.test")
	if _, err := h.GetCertificate(helloFor("mail.lerd.test")); err != nil {
		t.Fatal(err)
	}
}

var tlsState = tls.ConnectionState{}

func helloFor(name string) *tls.ClientHelloInfo { return &tls.ClientHelloInfo{ServerName: name} }

// A client behind another proxy keeps its chain, as $proxy_add_x_forwarded_for does.
func TestService_appendsToAnIncomingForwardedFor(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("X-Forwarded-For"))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	h := &Handler{Sites: func() []config.Site { return nil }, Services: func() []ServiceRoute {
		return []ServiceRoute{{Domain: "mail.lerd.test", Upstream: u.Host}}
	}}
	req := httptest.NewRequest("GET", "https://mail.lerd.test/", nil)
	req.TLS = &tlsState
	req.RemoteAddr = "10.0.0.9:1"
	req.Header.Set("X-Forwarded-For", "203.0.113.4")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Body.String(); got != "203.0.113.4, 10.0.0.9" {
		t.Fatalf("X-Forwarded-For %q", got)
	}
}
