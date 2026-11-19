// Package goserver is lerd's own web server, the alternative to nginx. It
// reads the site registry directly instead of rendered config files.
package goserver

import (
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/fcgi"
)

// Handler serves every registered site by Host, the way lerd's nginx vhosts do.
type Handler struct {
	// Sites returns the registered sites; read per request so a link or an
	// unlink is live without a reload.
	Sites func() []config.Site
	// Upstream is the host:port of the PHP-FPM serving a site.
	Upstream func(config.Site) string

	certs certCache
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	site, ok := h.siteFor(r.Host)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if site.Secured && r.TLS == nil {
		http.Redirect(w, r, httpsURL(r), http.StatusFound)
		return
	}
	h.servePHPSite(w, r, site)
}

// siteFor matches a host to the site that owns it or a subdomain of it, the
// longest domain winning, as nginx's exact and wildcard server_names do.
func (h *Handler) siteFor(hostport string) (config.Site, bool) {
	host := strings.ToLower(hostport)
	if hst, _, err := net.SplitHostPort(host); err == nil {
		host = hst
	}
	var best config.Site
	bestLen := 0
	for _, s := range h.Sites() {
		if s.Paused || s.Ignored {
			continue
		}
		for _, d := range s.Domains {
			if (host == d || strings.HasSuffix(host, "."+d)) && len(d) > bestLen {
				best, bestLen = s, len(d)
			}
		}
	}
	return best, bestLen > 0
}

// servePHPSite mirrors `try_files $uri $uri/ /index.php?$query_string`: a real
// file is sent as is, a PHP file or a directory's index.php runs itself, and
// anything else goes to the front controller.
func (h *Handler) servePHPSite(w http.ResponseWriter, r *http.Request, site config.Site) {
	root := filepath.Join(site.Path, config.PublicDirFor(site))
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	urlPath := path.Clean("/" + r.URL.Path)
	if hidesDotfile(urlPath) {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
		return
	}

	script := "/index.php"
	full := filepath.Join(root, filepath.FromSlash(urlPath))
	if fi, err := os.Stat(full); err == nil {
		switch {
		case fi.IsDir():
			if _, err := os.Stat(filepath.Join(full, "index.php")); err == nil {
				script = path.Join(urlPath, "index.php")
			}
		case strings.HasSuffix(urlPath, ".php"):
			script = urlPath
		default:
			http.ServeFile(w, r, full)
			return
		}
	}

	params := map[string]string{
		"SCRIPT_FILENAME": root + script,
		"SCRIPT_NAME":     script,
		"DOCUMENT_URI":    script,
		"DOCUMENT_ROOT":   root,
		"REQUEST_SCHEME":  scheme(r),
		"SERVER_PORT":     serverPort(r),
	}
	if err := fcgi.Serve(w, r, h.Upstream(site), params); err != nil {
		log.Printf("lerd web server: %s %s: %v", site.Name, r.URL.Path, err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
	}
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
