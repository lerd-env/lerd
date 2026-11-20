package goserver

import (
	"bufio"
	"bytes"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/pageinject"
	"github.com/geodro/lerd/internal/plugins"
)

// servePageEndpoints hands the browser log reporter and page plugin paths to
// lerd-ui with the site they came from, while the site has them switched on,
// as the vhost's own locations do. It reports whether it answered.
func (h *Handler) servePageEndpoints(w http.ResponseWriter, r *http.Request, site config.Site, branch string) bool {
	if h.WakeHold == nil {
		return false
	}
	limit := int64(0)
	switch {
	case (r.URL.Path == config.BrowserLogsPath || r.URL.Path == config.BrowserLogsPath+".js") && pageinject.BrowserLogs(site):
		limit = 64 << 10
	case strings.HasPrefix(r.URL.Path, plugins.PagePath) && len(pageinject.Plugins(site)) > 0:
		limit = 1 << 20
	default:
		return false
	}
	skipAccessFeed(w)
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(h.WakeHold)
			pr.Out.URL.Path, pr.Out.URL.RawPath = r.URL.Path, r.URL.RawPath
			pr.Out.Header.Set("X-Lerd-Site", site.Name)
			pr.Out.Header.Set("X-Lerd-Branch", branch)
			pr.Out.Header.Set("X-Lerd-Host", hostOnly(r.Host))
		},
		Transport: h.wakeTransport(),
	}
	rp.ServeHTTP(w, r)
	return true
}

// injecting wraps w so an HTML page gets the site's scripts in front of its
// first </head>, the request id PHP answered with filled in. Nil when the site
// injects nothing.
func injecting(w http.ResponseWriter, r *http.Request, site config.Site) *injector {
	if pageinject.Tags(site, "") == "" {
		return nil
	}
	// A compressed body cannot be rewritten, so the backend is asked for a
	// plain one, as the vhost's Accept-Encoding "" does.
	r.Header.Del("Accept-Encoding")
	if pageinject.BrowserLogs(site) {
		w.Header().Set("Access-Control-Expose-Headers", "X-Lerd-Rid")
	}
	return &injector{ResponseWriter: w, site: site}
}

type injector struct {
	http.ResponseWriter
	site               config.Site
	status             int
	decided, buffering bool
	buf                bytes.Buffer
}

func (in *injector) WriteHeader(code int) {
	if in.decided {
		return
	}
	in.decided, in.status = true, code
	ctype, _, _ := mime.ParseMediaType(in.Header().Get("Content-Type"))
	in.buffering = ctype == "text/html" && in.Header().Get("Content-Encoding") == ""
	if !in.buffering {
		in.ResponseWriter.WriteHeader(code)
	}
}

func (in *injector) Write(b []byte) (int, error) {
	if !in.decided {
		in.WriteHeader(http.StatusOK)
	}
	if in.buffering {
		return in.buf.Write(b)
	}
	return in.ResponseWriter.Write(b)
}

func (in *injector) Flush() {
	if !in.buffering {
		if f, ok := in.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// finish writes a buffered page with the tags in place, once, as sub_filter_once does.
func (in *injector) finish() {
	if !in.buffering {
		return
	}
	body := in.buf.Bytes()
	if i := bytes.Index(bytes.ToLower(body), []byte("</head>")); i >= 0 {
		tags := pageinject.Tags(in.site, in.Header().Get("X-Lerd-Rid"))
		body = append(append(append([]byte{}, body[:i]...), tags...), body[i:]...)
	}
	in.Header().Set("Content-Length", strconv.Itoa(len(body)))
	in.ResponseWriter.WriteHeader(in.status)
	in.ResponseWriter.Write(body)
}

// Hijack passes an upgrade straight through; there is no page to rewrite.
func (in *injector) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := in.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("lerd web server: %T cannot be hijacked", in.ResponseWriter)
	}
	in.decided = true
	return hj.Hijack()
}
