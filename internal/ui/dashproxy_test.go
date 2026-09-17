package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	// The preset decides, not the copy saved at install time: a service installed
	// before its dashboard moved behind the proxy must still be proxied, or the
	// store change only reaches people who reinstall.
	preflag := &config.CustomService{Name: "rabbitmq", Dashboard: "http://localhost:15672", DashboardExternal: false, Preset: "rabbitmq"}
	if !dashProxyEligible(preflag) {
		t.Error("dashboard_external must be read from the preset so existing installs pick the proxy up")
	}
	notExternal := &config.CustomService{Name: "elasticvue", Dashboard: "http://localhost:8083", DashboardExternal: true, Preset: "elasticvue"}
	if dashProxyEligible(notExternal) {
		t.Error("a preset whose dashboard is not external must not be proxied")
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
	p := newDashProxy("rabbitmq", target, dashProxyTweaks{})
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
	p := newDashProxy("rabbitmq", target, dashProxyTweaks{})
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/rabbitmq/api/whoami", nil)
	p.Director(req)
	if req.URL.Path != "/_svc/rabbitmq/api/whoami" {
		t.Errorf("URL.Path = %q, want the prefix forwarded unchanged (upstream is mounted there)", req.URL.Path)
	}
	if req.Host != "localhost:15672" {
		t.Errorf("Host = %q, want localhost:15672", req.Host)
	}
}

func TestDashProxyDirector_SendsProxyHeader(t *testing.T) {
	// pgAdmin learns its mount path from X-Script-Name on every request; without
	// it the upstream 404s the prefixed path and generates root-absolute URLs.
	target, _ := url.Parse("http://localhost:8081")
	p := newDashProxy("pgadmin", target, dashProxyTweaks{headerKey: "X-Script-Name", headerValue: "/_svc/pgadmin"})
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/pgadmin/browser/", nil)
	// A client-supplied value must not survive: it would move the mount and let a
	// crafted request drive the upstream to generate URLs pointing off the mount.
	req.Header.Set("X-Script-Name", "/attacker")
	p.Director(req)
	if got := req.Header.Get("X-Script-Name"); got != "/_svc/pgadmin" {
		t.Errorf("X-Script-Name = %q, want /_svc/pgadmin", got)
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
	p := newDashProxy("rabbitmq", target, dashProxyTweaks{})
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

func TestResolveDashboardURL_FollowsPublishedPortMove(t *testing.T) {
	svc := &config.CustomService{
		Name:      "redisinsight",
		Dashboard: "http://localhost:8085",
		Ports:     []string{"8085:5540"},
	}
	if got := resolveDashboardURL(svc, nil); got != "http://localhost:8085" {
		t.Errorf("no override should keep the default port, got %q", got)
	}
	moved := map[string]config.ServiceConfig{"redisinsight": {PublishedPort: 8090}}
	if got := resolveDashboardURL(svc, moved); got != "http://localhost:8090" {
		t.Errorf("proxy target must follow the port move, got %q", got)
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

// TestDashProxyDirector_StripsPrefixForUpstreamsThatCannotBeTold covers the
// upstream that has no setting for its mount path (Solr, the Mercure hub UI).
// The prefix is removed instead of forwarded, and the target's own base path
// carries what is left, so a relative-asset UI resolves under the mount.
func TestDashProxyDirector_StripsPrefixForUpstreamsThatCannotBeTold(t *testing.T) {
	target, _ := url.Parse("http://localhost:8983/solr/")
	p := newDashProxy("solr", target, dashProxyTweaks{stripPrefix: true})
	for _, tc := range []struct{ in, want string }{
		{"/_svc/solr/", "/solr/"},
		{"/_svc/solr/css/angular/common.css", "/solr/css/angular/common.css"},
		{"/_svc/solr/admin/cores", "/solr/admin/cores"},
		{"/_svc/solr", "/solr/"},
	} {
		req := httptest.NewRequest("GET", "http://lerd.localhost"+tc.in, nil)
		p.Director(req)
		if req.URL.Path != tc.want {
			t.Errorf("%s -> %q, want %q", tc.in, req.URL.Path, tc.want)
		}
	}
}

func TestDashProxyDirector_KeepsQueryWhenStripping(t *testing.T) {
	target, _ := url.Parse("http://localhost:8983/solr/")
	p := newDashProxy("solr", target, dashProxyTweaks{stripPrefix: true})
	req := httptest.NewRequest("GET", "http://lerd.localhost/_svc/solr/admin/cores?action=STATUS", nil)
	p.Director(req)
	if req.URL.RawQuery != "action=STATUS" {
		t.Errorf("RawQuery = %q, want action=STATUS", req.URL.RawQuery)
	}
}

// A redirect from a stripping upstream names its own base path, which the
// browser would resolve outside the mount.
func TestRewriteLocation_StripsUpstreamBasePath(t *testing.T) {
	got := rewriteLocationFrom("/solr/admin/", "localhost:8983", "/_svc/solr", "/solr/")
	if got != "/_svc/solr/admin/" {
		t.Errorf("Location = %q, want /_svc/solr/admin/", got)
	}
}

// A preset that only asks for the stripping mode must still be proxied, or the
// dashboard opens at a path its upstream does not serve.
func TestDashProxyEligible_StripOnlyPreset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	presets := filepath.Join(dir, "lerd", "service-presets")
	if err := os.MkdirAll(presets, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: solr\nimage: x\ndashboard: http://localhost:8983/solr/\ndashboard_proxy_strip: true\n"
	if err := os.WriteFile(filepath.Join(presets, "solr.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &config.CustomService{Name: "solr", Dashboard: "http://localhost:8983/solr/", Preset: "solr"}
	if !dashProxyEligible(svc) {
		t.Error("dashboard_proxy_strip must be enough to route through the proxy")
	}
	if !config.DashboardProxyStrips(svc) {
		t.Error("the stripping mode must be read back from the preset")
	}
}

// A page that cannot be told where it is mounted still asks for its assets at
// the origin root, which on the shared lerd-ui origin is lerd's own.
func TestRebaseDashboardHTML(t *testing.T) {
	const page = `<html><head><link rel=stylesheet href="/dist/app.css"><script src="/dist/app.js"></script>` +
		`<link rel=icon href="//cdn.example/x.svg"><a href="reports/1">rel</a></head>` +
		`<body><div id="app" data-webroot="/"></div></body></html>`
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:   io.NopCloser(strings.NewReader(page)),
	}
	if err := rebaseDashboardHTML(resp, "/_svc/mailpit", []string{"data-webroot"}); err != nil {
		t.Fatalf("rebase: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	got := string(out)
	for _, want := range []string{
		`href="/_svc/mailpit/dist/app.css"`,
		`src="/_svc/mailpit/dist/app.js"`,
		`data-webroot="/_svc/mailpit/"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	// Another origin's URL and a relative link are not ours to move.
	if !strings.Contains(got, `href="//cdn.example/x.svg"`) {
		t.Errorf("protocol-relative URL rebased: %s", got)
	}
	if !strings.Contains(got, `href="reports/1"`) {
		t.Errorf("relative link rebased: %s", got)
	}
	if resp.Header.Get("Content-Length") != strconv.Itoa(len(got)) {
		t.Errorf("Content-Length %q, body %d", resp.Header.Get("Content-Length"), len(got))
	}
}

func TestRebaseDashboardHTMLSkipsNonHTML(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(`{"href":"/api/v1"}`)),
	}
	if err := rebaseDashboardHTML(resp, "/_svc/mailpit", nil); err != nil {
		t.Fatalf("rebase: %v", err)
	}
	out, _ := io.ReadAll(resp.Body)
	if string(out) != `{"href":"/api/v1"}` {
		t.Errorf("JSON body rewritten: %s", out)
	}
}

// Mailpit is part of the default stack, so it has no service file; the proxy has
// to recognise it from the preset alone or the overlay embeds nothing.
func TestDefaultServiceIsProxyEligible(t *testing.T) {
	svc := config.DefaultPresetService("mailpit")
	if svc == nil {
		t.Fatal("no synthesised service for mailpit")
	}
	if !dashProxyEligible(svc) {
		t.Error("mailpit not eligible for the same-origin proxy")
	}
	tw := dashProxyTweaksFor(svc)
	if !tw.stripPrefix {
		t.Error("mailpit should be served by stripping the mount prefix")
	}
	if len(tw.rebaseAttrs) == 0 {
		t.Error("mailpit should rebase the base path it hands its own router")
	}
	if config.DefaultPresetService("mysql") != nil && config.DashboardProxied(config.DefaultPresetService("mysql")) {
		t.Error("a service with no dashboard should not be proxied")
	}
}

// An app that builds its API URLs from the origin it is served at reaches lerd's
// own root under the mount, which rebasing cannot fix: the URL does not exist
// until a script computes it.
func TestDashboardRerouteScript(t *testing.T) {
	svc := config.DefaultPresetService("meilisearch")
	if svc == nil {
		t.Fatal("no synthesised service for meilisearch")
	}
	tw := dashProxyTweaksFor(svc)
	if !tw.stripPrefix {
		t.Error("meilisearch should be served by stripping the mount prefix")
	}
	if !strings.Contains(tw.bootstrap, "/_svc/meilisearch") {
		t.Errorf("reroute script missing the mount: %q", tw.bootstrap)
	}
	for _, want := range []string{"window.fetch", "XMLHttpRequest.prototype.open"} {
		if !strings.Contains(tw.bootstrap, want) {
			t.Errorf("reroute script does not wrap %s", want)
		}
	}
}

// A dashboard served where its own build expects is matched by path rather than
// registered on the mux, so a service installed while lerd-ui runs is served
// without a restart.
func TestDashMountFor(t *testing.T) {
	svc, mount := dashMountFor("/rustfs/console/browser/")
	if svc == nil || svc.Name != "rustfs" {
		t.Fatalf("dashMountFor = %v, want rustfs", svc)
	}
	if mount != "/rustfs/console/" {
		t.Errorf("mount = %q, want /rustfs/console/", mount)
	}
	if got, _ := dashMountFor("/api/services"); got != nil {
		t.Errorf("lerd's own path claimed by %s", got.Name)
	}
	if got, _ := dashMountFor("/rustfs/console"); got == nil {
		t.Error("the mount root without its slash should still be served")
	}
}

// The mount answers at the upstream's own path, so cookies and redirects are
// scoped there and the browser's Host is forwarded for the signature to hold.
func TestDashMountTweaks(t *testing.T) {
	svc := config.DefaultPresetService("rustfs")
	tw := dashProxyTweaksFor(svc)
	if !tw.keepHost {
		t.Error("rustfs signs its requests; the Host must be forwarded as it arrived")
	}
	if !strings.Contains(tw.bootstrap, "#accessKey") {
		t.Error("the login form is not filled in")
	}
	if !strings.Contains(tw.bootstrap, "/rustfs/console/") {
		t.Error("the reroute does not leave the console's own path alone")
	}
}

func TestWithDashboardMountsPassesOtherPaths(t *testing.T) {
	served := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true })
	rec := httptest.NewRecorder()
	withDashboardMounts(next).ServeHTTP(rec, httptest.NewRequest("GET", "/api/services", nil))
	if !served {
		t.Error("a path no dashboard claims should reach the rest of lerd-ui")
	}
}
