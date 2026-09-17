package ui

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// Bundled admin dashboards set session cookies the browser refuses to carry
// into a cross-origin iframe. We serve them same-origin under /_svc/<name>/ so
// their cookies are first-party again, the same trick the SPX profiler uses
// under /_spx/ (see profiler.go). Each upstream is told to mount its UI at that
// same prefix, so the proxy forwards the path unchanged rather than stripping
// it: rabbitmq via management.path_prefix and phpmyadmin via an apache Alias
// (conf mounts), redisinsight and mongo-express via env (PresetProxyEnv), and
// pgadmin per request (PresetProxyHeader).

const dashProxyPrefix = config.DashboardProxyPrefix

// dashProxyPath is the same-origin mount path for a proxied service dashboard.
func dashProxyPath(name string) string {
	return config.DashboardProxyPath(name)
}

// dashProxyName extracts the service name from a /_svc/<name>/... request path.
func dashProxyName(p string) string {
	rest := strings.TrimPrefix(p, dashProxyPrefix)
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// dashProxyEligible reports whether a custom service should be served through
// the same-origin proxy rather than opened in a new tab. It shares the decision
// with the quadlet generator, which uses it to tell the upstream where it is
// mounted, so the two can't disagree about where the dashboard lives.
func dashProxyEligible(svc *config.CustomService) bool {
	return config.DashboardProxied(svc)
}

// dashProxyTweaks are the per-preset adjustments one proxied dashboard needs:
// a request header telling the upstream which path it is mounted at, and an
// inline <script> injected into its HTML so it opens authenticated. Both are
// derived from the preset, so they are stable for a given service name.
type dashProxyTweaks struct {
	headerKey   string
	headerValue string
	bootstrap   string
	// stripPrefix removes /_svc/<name> from the request before forwarding, for
	// an upstream that has no setting for its mount path. The target URL's own
	// path carries what is left, so a UI whose assets are path-relative (Solr,
	// the Mercure hub UI) resolves them under the mount without being told.
	stripPrefix bool
	// rebaseAttrs are the extra HTML attributes whose root-absolute values are
	// rewritten onto the mount alongside href and src, for a page that hands its
	// own router a base path in an attribute of its own.
	rebaseAttrs []string
}

// dashProxyTweaksFor collects what the proxy must add for a service's preset.
func dashProxyTweaksFor(svc *config.CustomService) dashProxyTweaks {
	tw := dashProxyTweaks{
		bootstrap:   config.PresetDashboardBootstrap(svc),
		stripPrefix: config.DashboardProxyStrips(svc),
		rebaseAttrs: config.DashboardProxyRebases(svc),
	}
	if k, v, ok := config.PresetProxyHeader(svc); ok {
		tw.headerKey, tw.headerValue = k, v
	}
	return tw
}

var (
	dashProxyMu    sync.Mutex
	dashProxyCache = map[string]*httputil.ReverseProxy{}
)

// dashProxyFor returns a cached reverse proxy for the named service, keyed by
// name and target so a changed dashboard URL rebuilds.
func dashProxyFor(name string, target *url.URL, tw dashProxyTweaks) *httputil.ReverseProxy {
	key := name + "|" + target.String()
	dashProxyMu.Lock()
	defer dashProxyMu.Unlock()
	if p, ok := dashProxyCache[key]; ok {
		return p
	}
	p := newDashProxy(name, target, tw)
	dashProxyCache[key] = p
	return p
}

// newDashProxy builds the reverse proxy that serves one bundled dashboard
// same-origin under /_svc/<name>/. The request path is forwarded unchanged
// because the upstream is mounted at the same prefix; the response is rewritten
// so it can be embedded in the lerd-ui iframe (strip framing headers, scope
// cookies and redirects to the mount path).
func newDashProxy(name string, target *url.URL, tw dashProxyTweaks) *httputil.ReverseProxy {
	prefix := strings.TrimSuffix(dashProxyPath(name), "/")
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Before the single-host director joins the target's base path: the
		// upstream never sees the mount, so what it gets is target.Path + the
		// remainder, which is the path it actually serves.
		if tw.stripPrefix {
			rest := strings.TrimPrefix(req.URL.Path, prefix)
			if rest == "" {
				rest = "/"
			}
			req.URL.Path = rest
		}
		orig(req)
		req.Host = target.Host
		// Set, not add: the mount path is ours to state, and a client-supplied
		// value would otherwise steer where the upstream thinks it lives.
		if tw.headerKey != "" {
			req.Header.Set(tw.headerKey, tw.headerValue)
		}
		// Preserve the scheme the browser actually used (nginx forwards it, and
		// lerd.localhost is served over https) so an upstream that builds absolute
		// URLs from X-Forwarded-Proto doesn't downgrade them to http. Default to
		// http only when nothing upstream told us otherwise.
		if proto := req.Header.Get("X-Forwarded-Proto"); proto != "http" && proto != "https" {
			// nginx (lerd.localhost) sets this from $scheme, overwriting any client
			// value; only an unexpected/injected value or a direct-to-socket client
			// reaches here, so recompute it from the connection rather than forward
			// whatever the client supplied.
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		// We rewrite the HTML to inject the auth bootstrap or to rebase its own
		// links, so ask the upstream for an uncompressed body we can edit.
		if tw.bootstrap != "" || tw.stripPrefix {
			req.Header.Del("Accept-Encoding")
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		// These admin UIs (or an upstream proxy) may forbid framing; lerd-ui
		// embeds them in an iframe, so drop the framing guards. Same intent as
		// pgadmin's X_FRAME_OPTIONS='' config mount, applied here upstream-agnostic.
		resp.Header.Del("X-Frame-Options")
		if csp := stripFrameAncestors(resp.Header.Get("Content-Security-Policy")); csp == "" {
			resp.Header.Del("Content-Security-Policy")
		} else {
			resp.Header.Set("Content-Security-Policy", csp)
		}
		rewriteSetCookiePaths(resp.Header, prefix+"/")
		if loc := resp.Header.Get("Location"); loc != "" {
			base := ""
			if tw.stripPrefix {
				base = target.Path
			}
			resp.Header.Set("Location", rewriteLocationFrom(loc, target.Host, prefix, base))
		}
		if tw.stripPrefix {
			if err := rebaseDashboardHTML(resp, prefix, tw.rebaseAttrs); err != nil {
				return err
			}
		}
		if tw.bootstrap != "" {
			return injectDashboardBootstrap(resp, tw.bootstrap)
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "dashboard upstream unavailable", http.StatusBadGateway)
	}
	return proxy
}

// isLoopbackTarget reports whether host is a loopback destination: the literal
// "localhost" or a loopback IP (127.0.0.0/8, ::1). A non-literal hostname is
// rejected rather than resolved, so DNS can't be used to slip past the gate.
func isLoopbackTarget(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// stripFrameAncestors removes the frame-ancestors directive from a CSP value so
// the dashboard can be embedded same-origin, leaving the rest of the policy
// intact. Returns the empty string when nothing else remains.
func stripFrameAncestors(csp string) string {
	if csp == "" {
		return ""
	}
	var kept []string
	for _, d := range strings.Split(csp, ";") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(d)), "frame-ancestors") {
			continue
		}
		if strings.TrimSpace(d) == "" {
			continue
		}
		kept = append(kept, strings.TrimSpace(d))
	}
	return strings.Join(kept, "; ")
}

// rewriteSetCookiePaths scopes root-path cookies to the proxy mount so cookies
// from different proxied dashboards don't collide on the shared lerd-ui origin.
// Cookies the upstream already scoped to a sub-path are left untouched.
func rewriteSetCookiePaths(h http.Header, mountPath string) {
	cookies := h["Set-Cookie"]
	for i, c := range cookies {
		cookies[i] = rewriteCookiePath(c, mountPath)
	}
}

func rewriteCookiePath(cookie, mountPath string) string {
	parts := strings.Split(cookie, ";")
	for i, p := range parts {
		t := strings.TrimSpace(p)
		lower := strings.ToLower(t)
		if !strings.HasPrefix(lower, "path=") {
			continue
		}
		val := strings.TrimSpace(t[len("path="):])
		if val == "/" || val == "" {
			parts[i] = " Path=" + mountPath
		}
		return strings.Join(parts, ";")
	}
	return cookie + "; Path=" + mountPath
}

// rewriteLocation ensures a redirect issued by the upstream lands back inside
// the /_svc/<name> mount instead of escaping to the lerd-ui root or the
// upstream's own host.
func rewriteLocation(loc, targetHost, prefix string) string {
	return rewriteLocationFrom(loc, targetHost, prefix, "")
}

// rewriteLocationFrom is rewriteLocation for a stripping mount, where the
// upstream names its own base path (/solr/...) in the redirect and the browser
// would follow it out of the mount. basePath is empty for a forwarding mount.
func rewriteLocationFrom(loc, targetHost, prefix, basePath string) string {
	if u, err := url.Parse(loc); err == nil && u.Host != "" {
		if u.Host != targetHost {
			// Off-origin redirect (a foreign absolute URL, or a scheme-relative
			// //host that the browser would follow away from the iframe). These
			// local admin dashboards never legitimately redirect off-host, so
			// neutralize the escape by keeping the browser inside the mount.
			return prefix + "/"
		}
		loc = u.EscapedPath()
		if loc == "" {
			// A redirect to the upstream's bare origin (no path) must map to the
			// mount root, not an empty Location that the browser resolves to a
			// no-op reload of the current page.
			loc = "/"
		}
		if u.RawQuery != "" {
			loc += "?" + u.RawQuery
		}
	}
	if !strings.HasPrefix(loc, "/") {
		return loc
	}
	if loc == prefix || strings.HasPrefix(loc, prefix+"/") {
		return loc
	}
	if basePath != "" && basePath != "/" {
		trimmed := strings.TrimSuffix(basePath, "/")
		if loc == trimmed {
			return prefix + "/"
		}
		loc = strings.TrimPrefix(loc, trimmed)
	}
	return prefix + loc
}

// rebaseDashboardHTML moves a page's own root-absolute links onto the mount it
// is served at. Stripping the prefix lets an upstream that cannot be told where
// it lives answer at all, but its page still asks for /dist/app.css, which on
// the shared lerd-ui origin is lerd's own root rather than the upstream's.
// Attributes whose value does not start with a single slash are left alone, so
// a relative link, an absolute URL and a protocol-relative one all pass through.
func rebaseDashboardHTML(resp *http.Response, prefix string, extra []string) error {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	html := string(body)
	for _, attr := range append([]string{"href", "src"}, extra...) {
		html = rebaseAttribute(html, attr, prefix)
	}
	resp.Body = io.NopCloser(strings.NewReader(html))
	resp.ContentLength = int64(len(html))
	resp.Header.Set("Content-Length", strconv.Itoa(len(html)))
	resp.Header.Del("Content-Encoding")
	return nil
}

// rebaseAttribute prefixes every root-absolute value of one HTML attribute. A
// protocol-relative URL starts with two slashes and is another origin's, so the
// second character decides.
func rebaseAttribute(html, attr, prefix string) string {
	var b strings.Builder
	rest := html
	for {
		i := strings.Index(rest, attr+`="/`)
		if i < 0 {
			break
		}
		cut := i + len(attr) + 2
		b.WriteString(rest[:cut])
		if strings.HasPrefix(rest[cut:], "//") {
			rest = rest[cut:]
			continue
		}
		b.WriteString(prefix)
		rest = rest[cut:]
	}
	b.WriteString(rest)
	return b.String()
}

// injectDashboardBootstrap rewrites an HTML dashboard response to insert the
// auth bootstrap script right after <head>, so it runs before the app's own
// scripts and the dashboard opens already logged in. Non-HTML responses (assets,
// API) pass through untouched.
func injectDashboardBootstrap(resp *http.Response, script string) error {
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	html := string(body)
	if i := strings.Index(html, "<head>"); i >= 0 {
		html = html[:i+len("<head>")] + script + html[i+len("<head>"):]
	} else {
		html = script + html
	}
	resp.Body = io.NopCloser(strings.NewReader(html))
	resp.ContentLength = int64(len(html))
	resp.Header.Set("Content-Length", strconv.Itoa(len(html)))
	resp.Header.Del("Content-Encoding")
	return nil
}

// resolveDashboardURL returns the dashboard upstream a proxied service should
// target. The stored dashboard URL keeps the preset's default host port; a user
// port move is recorded in the service config, not rewritten into svc.Dashboard,
// so follow the move the same way the dashboard link does, or the proxy keeps
// dialing the stale port after the container rebinds.
func resolveDashboardURL(svc *config.CustomService, services map[string]config.ServiceConfig) string {
	return serviceops.WithDashboardPort(svc.Dashboard, svc.Ports, services[svc.Name])
}

// handleDashProxy serves a bundled service dashboard same-origin under
// /_svc/<name>/. It requires dashboard-control authority.
func handleDashProxy(w http.ResponseWriter, r *http.Request) {
	if !hasHostActionAuthority(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	// The global CSRF gate trusts the unix socket unconditionally, but every
	// /_svc/ request arrives over it (the lerd.localhost vhost proxies to the
	// socket), so that trust alone would forward a cross-origin request straight
	// into the third-party admin API (RabbitMQ management, RedisInsight) with the
	// dashboard's first-party cookies attached. Re-apply the cross-origin check
	// here without the socket bypass: the dashboard is embedded same-origin, so
	// its own traffic is same-origin/none; reject a cross-site or same-site
	// initiator. A missing header (host tooling, old clients) keeps loopback trust.
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	name := dashProxyName(r.URL.Path)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	svc, err := config.LoadCustomService(name)
	if err != nil {
		// A default-stack service has no service file to load; it is the preset.
		svc = config.DefaultPresetService(name)
	}
	if svc == nil || !dashProxyEligible(svc) {
		http.NotFound(w, r)
		return
	}
	target, err := url.Parse(resolveDashboardURL(svc, loadServicesMap()))
	if err != nil || target.Host == "" {
		http.Error(w, fmt.Sprintf("invalid dashboard URL for %s", name), http.StatusBadGateway)
		return
	}
	// The dashboard URL comes from a user-writable service file; every bundled
	// preset points at a loopback admin UI. Refuse anything else so the proxy
	// can't be coerced into reaching an arbitrary host (cloud metadata, internal
	// services) with the dashboard's injected credentials attached.
	if !isLoopbackTarget(target.Hostname()) {
		http.Error(w, fmt.Sprintf("dashboard for %s must be loopback", name), http.StatusBadGateway)
		return
	}
	dashProxyFor(name, target, dashProxyTweaksFor(svc)).ServeHTTP(w, r)
}
