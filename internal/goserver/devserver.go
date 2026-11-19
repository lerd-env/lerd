package goserver

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteproxy"
)

var errRetryUnprefixed = errors.New("dev server 404: retry without the prefix")

// serveDevServer sends the site's dev-server prefix to its dev server on the
// host, as the vhost's `location ^~ <base>` does. It reports false when the
// request is not under the prefix.
func (h *Handler) serveDevServer(w http.ResponseWriter, r *http.Request, site config.Site) bool {
	if h.WorkerHost == nil {
		return false
	}
	base, port := siteproxy.DevServer(site)
	if base == "" || !strings.HasPrefix(r.URL.Path, base) {
		return false
	}
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(h.WorkerHost(site, true), strconv.Itoa(port))}
	first := devProxy(r, target, func(w http.ResponseWriter, req *http.Request, err error) {
		if errors.Is(err, errRetryUnprefixed) {
			// Plugin middleware ahead of the tool's base handling never sees the
			// prefix stripped, so the vhost retries a 404 once without it.
			retry := req.Clone(req.Context())
			retry.URL.Path = "/" + strings.TrimPrefix(req.URL.Path, base)
			retry.URL.RawPath = ""
			devProxy(retry, target, devDown(port)).ServeHTTP(w, retry)
			return
		}
		devDown(port)(w, req, err)
	})
	first.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return errRetryUnprefixed
		}
		return nil
	}
	first.ServeHTTP(w, r)
	return true
}

func devProxy(r *http.Request, target *url.URL, onError func(http.ResponseWriter, *http.Request, error)) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = r.Host
			pr.Out.Header.Set("X-Forwarded-Proto", scheme(r))
		},
		FlushInterval: -1,
		ErrorHandler:  onError,
	}
}

// devDown is the vhost's @lerd_dev_down: a readable 502 any origin may read,
// since the page loading these assets may sit on another of the site's
// domains and an unreadable error reaches its console as a CORS failure.
func devDown(port int) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("lerd web server: dev server :%d: %v", port, err)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, "lerd: nothing is listening on this site's dev server port (%d). Its worker is stopped or its server has died; start it from the dashboard or see lerd worker list.\n", port)
	}
}
