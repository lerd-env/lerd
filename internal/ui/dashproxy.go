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
)

// Bundled admin dashboards (rabbitmq, redisinsight) set session/consent cookies
// the browser refuses to carry into a cross-origin iframe, and their upstreams
// expose no SameSite knob the way pgadmin/phpmyadmin do. We serve them
// same-origin under /_svc/<name>/ so their cookies are first-party again, the
// same trick the SPX profiler uses under /_spx/ (see profiler.go). The upstream
// apps are configured to mount their UI at the same /_svc/<name> prefix
// (rabbitmq management.path_prefix, redisinsight RI_PROXY_PATH), so the proxy
// forwards the path unchanged rather than stripping it.

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
// the same-origin proxy rather than opened in a new tab. Only bundled presets
// (Preset set and still resolvable) that asked for an external dashboard
// qualify; a user-defined custom service with dashboard_external keeps the
// new-tab behavior.
func dashProxyEligible(svc *config.CustomService) bool {
	if svc == nil || !svc.DashboardExternal || svc.Dashboard == "" || svc.Preset == "" {
		return false
	}
	_, err := config.LoadPreset(svc.Preset)
	return err == nil
}

var (
	dashProxyMu    sync.Mutex
	dashProxyCache = map[string]*httputil.ReverseProxy{}
)

// dashProxyFor returns a cached reverse proxy for the named service, keyed by
// name and target so a changed dashboard URL rebuilds. bootstrap is an inline
// <script> injected into the dashboard's HTML so it opens authenticated.
func dashProxyFor(name string, target *url.URL, bootstrap string) *httputil.ReverseProxy {
	key := name + "|" + target.String()
	dashProxyMu.Lock()
	defer dashProxyMu.Unlock()
	if p, ok := dashProxyCache[key]; ok {
		return p
	}
	p := newDashProxy(name, target, bootstrap)
	dashProxyCache[key] = p
	return p
}

// newDashProxy builds the reverse proxy that serves one bundled dashboard
// same-origin under /_svc/<name>/. The request path is forwarded unchanged
// because the upstream is mounted at the same prefix; the response is rewritten
// so it can be embedded in the lerd-ui iframe (strip framing headers, scope
// cookies and redirects to the mount path).
func newDashProxy(name string, target *url.URL, bootstrap string) *httputil.ReverseProxy {
	prefix := strings.TrimSuffix(dashProxyPath(name), "/")
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		req.Host = target.Host
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
		// We rewrite the HTML to inject the auth bootstrap, so ask the upstream
		// for an uncompressed body we can edit.
		if bootstrap != "" {
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
			resp.Header.Set("Location", rewriteLocation(loc, target.Host, prefix))
		}
		if bootstrap != "" {
			return injectDashboardBootstrap(resp, bootstrap)
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
	return prefix + loc
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

// handleDashProxy serves a bundled service dashboard same-origin under
// /_svc/<name>/. Loopback-only, since it forwards into a local admin UI.
func handleDashProxy(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
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
	if err != nil || !dashProxyEligible(svc) {
		http.NotFound(w, r)
		return
	}
	target, err := url.Parse(svc.Dashboard)
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
	dashProxyFor(name, target, config.PresetDashboardBootstrap(svc)).ServeHTTP(w, r)
}
