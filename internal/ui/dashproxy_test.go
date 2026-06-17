package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestDashProxyPathAndName(t *testing.T) {
	if got := dashProxyPath("rabbitmq"); got != "/_svc/rabbitmq/" {
		t.Errorf("dashProxyPath = %q, want /_svc/rabbitmq/", got)
	}
	cases := map[string]string{
		"/_svc/rabbitmq/":        "rabbitmq",
		"/_svc/rabbitmq":         "rabbitmq",
		"/_svc/redisinsight/api": "redisinsight",
		"/_svc/":                 "",
	}
	for in, want := range cases {
		if got := dashProxyName(in); got != want {
			t.Errorf("dashProxyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDashProxyEligible(t *testing.T) {
	bundled := &config.CustomService{Name: "rabbitmq", Dashboard: "http://localhost:15672", DashboardExternal: true, Preset: "rabbitmq"}
	if !dashProxyEligible(bundled) {
		t.Error("bundled preset with dashboard_external should be proxy-eligible")
	}
	userCustom := &config.CustomService{Name: "myadmin", Dashboard: "http://localhost:9000", DashboardExternal: true, Preset: ""}
	if dashProxyEligible(userCustom) {
		t.Error("user custom service (no preset) must keep the new-tab behavior")
	}
	notExternal := &config.CustomService{Name: "rabbitmq", Dashboard: "http://localhost:15672", DashboardExternal: false, Preset: "rabbitmq"}
	if dashProxyEligible(notExternal) {
		t.Error("service without dashboard_external should not be proxied")
	}
	unknownPreset := &config.CustomService{Name: "x", Dashboard: "http://localhost:1", DashboardExternal: true, Preset: "does-not-exist"}
	if dashProxyEligible(unknownPreset) {
		t.Error("service referencing an unresolvable preset should not be proxied")
	}
}

func TestStripFrameAncestors(t *testing.T) {
	cases := map[string]string{
		"frame-ancestors 'none'":                     "",
		"default-src 'self'; frame-ancestors 'none'": "default-src 'self'",
		"frame-ancestors 'none'; default-src 'self'": "default-src 'self'",
		"default-src 'self'":                         "default-src 'self'",
		"":                                           "",
	}
	for in, want := range cases {
		if got := stripFrameAncestors(in); got != want {
			t.Errorf("stripFrameAncestors(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteCookiePath(t *testing.T) {
	cases := map[string]string{
		"sid=abc; Path=/; HttpOnly":      "sid=abc; Path=/_svc/rabbitmq/; HttpOnly",
		"sid=abc; HttpOnly":              "sid=abc; HttpOnly; Path=/_svc/rabbitmq/",
		"sid=abc; Path=/_svc/rabbitmq/x": "sid=abc; Path=/_svc/rabbitmq/x",
	}
	for in, want := range cases {
		if got := rewriteCookiePath(in, "/_svc/rabbitmq/"); got != want {
			t.Errorf("rewriteCookiePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteLocation(t *testing.T) {
	cases := map[string]string{
		"/":                            "/_svc/rabbitmq/",
		"/login":                       "/_svc/rabbitmq/login",
		"/_svc/rabbitmq/already":       "/_svc/rabbitmq/already",
		"http://localhost:15672/login": "/_svc/rabbitmq/login",
		// Off-origin redirects are neutralized to the mount root so the upstream
		// can't bounce the iframe off-site (foreign absolute URL or scheme-relative).
		"https://elsewhere.test/x": "/_svc/rabbitmq/",
		"//evil.com/path":          "/_svc/rabbitmq/",
		// A redirect to the upstream's bare origin must land at the mount root,
		// not become an empty Location (no-op reload).
		"http://localhost:15672":          "/_svc/rabbitmq/",
		"http://localhost:15672?next=/ui": "/_svc/rabbitmq/?next=/ui",
	}
	for in, want := range cases {
		if got := rewriteLocation(in, "localhost:15672", "/_svc/rabbitmq"); got != want {
			t.Errorf("rewriteLocation(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDashProxyModifyResponse(t *testing.T) {
	target, _ := url.Parse("http://localhost:15672")
	p := newDashProxy("rabbitmq", target, "")
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("X-Frame-Options", "DENY")
	resp.Header.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	resp.Header.Set("Set-Cookie", "sid=abc; Path=/; HttpOnly")
	resp.Header.Set("Location", "/login")

	if err := p.ModifyResponse(resp); err != nil {
		t.Fatalf("ModifyResponse: %v", err)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options = %q, want it stripped so the dashboard can be framed", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("CSP = %q, want frame-ancestors removed", got)
	}
	if got := resp.Header.Get("Set-Cookie"); got != "sid=abc; Path=/_svc/rabbitmq/; HttpOnly" {
		t.Errorf("Set-Cookie = %q, want Path scoped to the mount", got)
	}
	if got := resp.Header.Get("Location"); got != "/_svc/rabbitmq/login" {
		t.Errorf("Location = %q, want it prefixed with the mount", got)
	}
}

func TestInjectDashboardBootstrap(t *testing.T) {
	script := "<script>SEED</script>"
	resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<html><head><title>x</title></head><body></body></html>"))}
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Header.Set("Content-Encoding", "gzip")
	if err := injectDashboardBootstrap(resp, script); err != nil {
		t.Fatalf("inject: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	if got := string(out); got != "<html><head><script>SEED</script><title>x</title></head><body></body></html>" {
		t.Errorf("injected HTML wrong:\n%s", got)
	}
	if resp.Header.Get("Content-Length") == "" || resp.Header.Get("Content-Encoding") != "" {
		t.Errorf("must reset Content-Length and drop stale Content-Encoding, got len=%q enc=%q",
			resp.Header.Get("Content-Length"), resp.Header.Get("Content-Encoding"))
	}

	js := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader("var x=1"))}
	js.Header.Set("Content-Type", "application/javascript")
	if err := injectDashboardBootstrap(js, script); err != nil {
		t.Fatalf("inject js: %v", err)
	}
	if out, _ := io.ReadAll(js.Body); string(out) != "var x=1" {
		t.Errorf("non-HTML response must not be modified, got %q", out)
	}
}

func TestDashProxyDirector_ForwardsPathAndSetsHost(t *testing.T) {
	target, _ := url.Parse("http://localhost:15672")
	p := newDashProxy("rabbitmq", target, "")
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/rabbitmq/api/whoami", nil)
	p.Director(req)
	if req.URL.Path != "/_svc/rabbitmq/api/whoami" {
		t.Errorf("URL.Path = %q, want the prefix forwarded unchanged (upstream is mounted there)", req.URL.Path)
	}
	if req.Host != "localhost:15672" {
		t.Errorf("Host = %q, want localhost:15672", req.Host)
	}
}

func TestHandleDashProxy_RejectsNonLoopback(t *testing.T) {
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/rabbitmq/", nil)
	req.RemoteAddr = "203.0.113.7:9999"
	rec := httptest.NewRecorder()
	handleDashProxy(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a non-loopback request", rec.Code)
	}
}

func TestHandleDashProxy_RejectsCrossOrigin(t *testing.T) {
	// A loopback request (over the lerd.localhost unix socket) that a malicious
	// .test page initiated cross-site must be refused before it reaches the admin
	// upstream, even though the global CSRF gate trusts the socket.
	for _, site := range []string{"cross-site", "same-site"} {
		req := httptest.NewRequest("POST", "http://lerd.localhost/_svc/rabbitmq/api/queues", nil)
		req.RemoteAddr = "127.0.0.1:5050"
		req.Header.Set("Sec-Fetch-Site", site)
		rec := httptest.NewRecorder()
		handleDashProxy(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("Sec-Fetch-Site=%s: status = %d, want 403", site, rec.Code)
		}
	}
}

func TestIsLoopbackTarget(t *testing.T) {
	cases := map[string]bool{
		"localhost":       true,
		"LocalHost":       true,
		"127.0.0.1":       true,
		"127.5.6.7":       true,
		"::1":             true,
		"169.254.169.254": false,
		"10.0.0.5":        false,
		"evil.com":        false,
		"":                false,
	}
	for host, want := range cases {
		if got := isLoopbackTarget(host); got != want {
			t.Errorf("isLoopbackTarget(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestDashProxyDirector_RecomputesInjectedForwardedProto(t *testing.T) {
	target, _ := url.Parse("http://localhost:15672")
	p := newDashProxy("rabbitmq", target, "")
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/rabbitmq/", nil)
	req.Header.Set("X-Forwarded-Proto", "https\nX-Injected: 1")
	p.Director(req)
	if got := req.Header.Get("X-Forwarded-Proto"); got != "http" {
		t.Errorf("X-Forwarded-Proto = %q, want it recomputed to http (no TLS) when the inbound value is not http/https", got)
	}

	// A legitimate nginx-set https value is preserved.
	req2 := httptest.NewRequest("GET", "http://lerd.localhost/_svc/rabbitmq/", nil)
	req2.Header.Set("X-Forwarded-Proto", "https")
	p.Director(req2)
	if got := req2.Header.Get("X-Forwarded-Proto"); got != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want the nginx-set https preserved", got)
	}
}

func TestHandleDashProxy_UnknownService404(t *testing.T) {
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/not-installed-xyz/", nil)
	req.RemoteAddr = "127.0.0.1:5050"
	rec := httptest.NewRecorder()
	handleDashProxy(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown/ineligible service", rec.Code)
	}
}
