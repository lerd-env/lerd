package goserver

import (
	"log"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/webcustom"
)

// applyCustom applies the site's own headers, body cap, redirects and extra
// proxies, and reports whether it answered the request itself.
func applyCustom(w http.ResponseWriter, r *http.Request, c webcustom.Custom) bool {
	for k, v := range c.Headers {
		w.Header().Set(k, v)
	}
	if c.MaxBodySet && c.MaxBodyBytes > 0 {
		if r.ContentLength > c.MaxBodyBytes {
			http.Error(w, "413 Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return true
		}
		r.Body = http.MaxBytesReader(w, r.Body, c.MaxBodyBytes)
	}
	urlPath := path.Clean("/" + r.URL.Path)
	for _, rd := range c.Redirects {
		if under(urlPath, rd.From) {
			status := rd.Status
			if status == 0 {
				status = http.StatusFound
			}
			http.Redirect(w, r, rd.To, status)
			return true
		}
	}
	for _, p := range c.Proxies {
		if under(urlPath, p.Path) {
			target, _ := url.Parse(p.To) // validated when the file was read
			serveWorker(w, r, target)
			return true
		}
	}
	return false
}

// customRoot is the directory a configured root serves urlPath from.
func customRoot(c webcustom.Custom, urlPath, project, public string) (string, bool) {
	for _, rt := range c.Roots {
		if under(urlPath, rt.Path) {
			dir := webcustom.ExpandRoot(rt.Root, project, public)
			if real, err := filepath.EvalSymlinks(dir); err == nil {
				dir = real
			}
			return dir, true
		}
	}
	return "", false
}

// under is nginx's ^<prefix>(/|$): the path itself or anything below it.
func under(p, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	return prefix == "" || p == prefix || strings.HasPrefix(p, prefix+"/")
}

// frameworkRoots are the roots the site's framework declares in the store. A
// value this binary cannot serve is refused by name, never guessed at.
func frameworkRoots(site config.Site) webcustom.Custom {
	fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
	if !ok || fw.Web == nil {
		return webcustom.Custom{}
	}
	var c webcustom.Custom
	for _, r := range fw.Web.Roots {
		// Only a directory inside the project, so a definition can never point
		// the web server at the rest of the machine.
		inProject := strings.HasPrefix(r.Root, "{{root}}") || strings.HasPrefix(r.Root, "{{public}}")
		if !strings.HasPrefix(r.Path, "/") || !inProject || strings.Contains(r.Root, "..") ||
			strings.Contains(webcustom.ExpandRoot(r.Root, "", ""), "{{") {
			log.Printf("lerd web server: %s framework root %q -> %q is not one this lerd can serve, skipped", site.Framework, r.Path, r.Root)
			continue
		}
		c.Roots = append(c.Roots, webcustom.Root{Path: r.Path, Root: r.Root})
	}
	return c
}
