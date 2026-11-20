// Package goserver is lerd's own web server, the alternative to nginx. It
// reads the site registry directly instead of rendered config files.
package goserver

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/fcgi"
	"github.com/geodro/lerd/internal/sitephp"
	"github.com/geodro/lerd/internal/webcustom"
)

// Handler serves every registered site by Host, the way lerd's nginx vhosts do.
type Handler struct {
	// Sites returns the registered sites; read per request so a link or an
	// unlink is live without a reload.
	Sites func() []config.Site
	// Upstream is the host:port of the PHP-FPM serving a site.
	Upstream func(config.Site) string
	// Proxy returns where a site served by its own process (a container, a
	// FrankenPHP worker, a dev server on the host) is proxied to; ok is false
	// for a PHP-FPM site.
	Proxy func(config.Site) (target *url.URL, ok bool)
	// Services returns the service domains to serve, read per request.
	Services func() []ServiceRoute
	// WorkerHost is where a site's worker server listens: its PHP container,
	// or the host for a worker that runs there.
	WorkerHost func(site config.Site, onHost bool) string
	// WakeHold is lerd-ui's base URL, where a sleeping site's requests wait,
	// reached through WakeTransport (its unix socket on Linux) when set.
	WakeHold      *url.URL
	WakeTransport http.RoundTripper
	// ProfilerUpstream is the PHP-FPM serving the SPX UI and the bridge
	// script it runs there.
	ProfilerUpstream func() (addr, script string)

	certs certCache
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if hostOnly(strings.ToLower(r.Host)) == dashboardHost {
		h.serveDashboard(w, r)
		return
	}
	if hostOnly(strings.ToLower(r.Host)) == profilerHost {
		h.serveProfiler(w, r)
		return
	}
	// A service domain is an exact name, which outranks a site's wildcard.
	if svc, ok := h.serviceFor(r.Host); ok {
		serveService(w, r, svc)
		return
	}
	site, domain, ok := h.siteFor(r.Host)
	if !ok {
		serveUnknownHost(w)
		return
	}
	if site.Secured && r.TLS == nil {
		http.Redirect(w, r, httpsURL(r), http.StatusFound)
		return
	}
	if site.Paused {
		servePaused(w)
		return
	}
	if h.WakeHold != nil && Waking(site) {
		h.serveWakeHold(w, r)
		return
	}
	if target, ok := h.workerSubdomain(site, domain, r.Host); ok {
		serveProxied(w, r, target)
		return
	}
	branch := ""
	if wt, wtBranch, ok := worktreeSite(site, domain, r.Host); ok {
		site, branch = wt, wtBranch
	}
	customDomain := site.PrimaryDomain()
	if branch != "" {
		customDomain = strings.ToLower(hostOnly(r.Host))
	}
	custom, err := webcustom.For(customDomain)
	if err != nil {
		log.Printf("lerd web server: %v", err)
		http.Error(w, "lerd: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if applyCustom(w, r, custom) {
		return
	}
	if h.servePageEndpoints(w, r, site, branch) {
		return
	}
	if in := injecting(w, r, site); in != nil {
		defer in.finish()
		w = in
	}
	if h.serveDevServer(w, r, site) {
		return
	}
	if h.Proxy != nil {
		if target, ok := h.Proxy(site); ok {
			serveProxied(w, r, target)
			return
		}
	}
	h.servePHPSite(w, r, site, branch, custom)
}

// siteFor matches a host to the site that owns it or a subdomain of it, the
// longest domain winning, as nginx's exact and wildcard server_names do.
func (h *Handler) siteFor(hostport string) (config.Site, string, bool) {
	host := strings.ToLower(hostport)
	if hst, _, err := net.SplitHostPort(host); err == nil {
		host = hst
	}
	var best config.Site
	bestDomain := ""
	for _, s := range h.Sites() {
		if s.Ignored {
			continue
		}
		for _, d := range s.Domains {
			if (host == d || strings.HasSuffix(host, "."+d)) && len(d) > len(bestDomain) {
				best, bestDomain = s, d
			}
		}
	}
	return best, bestDomain, bestDomain != ""
}

// servePHPSite mirrors `try_files $uri $uri/ /index.php?$query_string`: a real
// file is sent as is, a PHP file or a directory's index.php runs itself, and
// anything else goes to the front controller.
func (h *Handler) servePHPSite(w http.ResponseWriter, r *http.Request, site config.Site, branch string, custom webcustom.Custom) {
	publicDir := config.PublicDirFor(site)
	root := filepath.Join(site.Path, publicDir)
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	urlPath := path.Clean("/" + r.URL.Path)
	if target, ok := h.workerPath(site, urlPath); ok {
		serveWorker(w, r, target)
		return
	}
	if hidesDotfile(urlPath) {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
		return
	}
	frontController := sitephp.FrontController(site)
	if alt, ok := customRoot(custom, urlPath, site.Path, root); ok {
		root, frontController = alt, false
	} else if alt, ok := customRoot(frameworkRoots(site), urlPath, site.Path, root); ok {
		root, frontController = alt, false
	}
	script, pathInfo, static := phpScript(root, urlPath, frontController)
	if static != "" {
		http.ServeFile(w, r, static)
		return
	}

	params := map[string]string{
		"SCRIPT_FILENAME": root + script,
		"SCRIPT_NAME":     script,
		"DOCUMENT_URI":    script,
		"DOCUMENT_ROOT":   root,
		"PATH_INFO":       pathInfo,
		"REQUEST_SCHEME":  scheme(r),
		"SERVER_PORT":     serverPort(r),
	}
	for k, v := range lerdParams(r, site) {
		params[k] = v
	}
	if branch != "" {
		params["LERD_BRANCH"] = branch
	}
	if fw, ok := config.GetFrameworkForDir(site.Framework, site.Path); ok && fw.Nginx != nil {
		kept, _ := sitephp.FastCGIParams(site.Framework, site.PrimaryDomain(), fw.Nginx.FastCGIParams, site.Path, publicDir)
		for _, p := range kept {
			params[p.Name] = p.Value
		}
	}
	for k, v := range custom.FastCGIParams {
		params[k] = v
	}
	timeout := time.Duration(sitephp.RequestTimeout(site.Path, site.PHPVersion)) * time.Second
	if err := fcgi.Serve(w, r, h.Upstream(site), params, timeout); err != nil {
		log.Printf("lerd web server: %s %s: %v", site.Name, r.URL.Path, err)
		if errors.Is(err, fcgi.ErrTimeout) {
			http.Error(w, "504 Gateway Timeout", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
	}
}

// phpScript picks what serves urlPath as the vhost's locations do: a static
// file as is, a .php path or directory index as the script, else index.php.
// Front-controller routing sends all PHP to /index.php, splitting PATH_INFO.
func phpScript(root, urlPath string, frontController bool) (script, pathInfo, static string) {
	if frontController {
		if rest, ok := strings.CutPrefix(urlPath, "/index.php"); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
			return "/index.php", rest, ""
		}
	}
	full := filepath.Join(root, filepath.FromSlash(urlPath))
	fi, err := os.Stat(full)
	switch {
	case err == nil && fi.IsDir():
		if _, err := os.Stat(filepath.Join(full, "index.php")); err == nil && !frontController {
			return path.Join(urlPath, "index.php"), "", ""
		}
	case strings.HasSuffix(urlPath, ".php"):
		if frontController {
			return "/index.php", "", ""
		}
		return urlPath, "", ""
	case err == nil:
		return "", "", full
	}
	return "/index.php", "", ""
}

// hidesDotfile matches nginx's deny on dot paths, except /.well-known.
func hidesDotfile(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") && seg != ".well-known" {
			return true
		}
	}
	return false
}

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func serverPort(r *http.Request) string {
	if _, p, err := net.SplitHostPort(r.Host); err == nil {
		return p
	}
	if r.TLS != nil {
		return "443"
	}
	return strconv.Itoa(80)
}
