package cli

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// shareStorage is the bucket a site keeps on one of lerd's services, the one
// loopback port a public share forwards.
type shareStorage struct {
	bucket string
	// port is the loopback port the service is published on.
	port string
	// host is the one the app writes the bucket's file URLs with, which a
	// presigned URL's signature covers: the loopback address or a service domain.
	host string
	// domain matches URLs on the service's own domain; nil for a loopback host.
	domain *regexp.Regexp
}

// siteShareStorage reads the site's bucket from its framework's env file, under
// the keys a service mapping fills with the bucket and its URL. Nil when the
// bucket is not local, since a real one is reachable from anywhere.
func siteShareStorage(sitePath string) *shareStorage {
	fwName, ok := config.DetectFrameworkForDir(sitePath)
	if !ok {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(fwName, sitePath)
	if !ok {
		return nil
	}
	bucketKey, urlKey := bucketEnvKeys(fw.Env.Services)
	rel, format := frameworkEnvFile(sitePath)
	env := envfile.Values(filepath.Join(sitePath, rel), format)
	bucket := env[bucketKey]
	u, err := url.Parse(env[urlKey])
	if bucket == "" || err != nil || u.Host == "" {
		return nil
	}
	if strings.Trim(u.Path, "/") != bucket {
		return nil // a virtual-host or prefixed layout the prefix doesn't map
	}
	s := &shareStorage{bucket: bucket, host: u.Host, port: u.Port()}
	// only the loopback hosts the rewriter matches, or the URLs would stay local
	if slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, u.Hostname()) {
		if s.port == "" {
			return nil
		}
		return s
	}
	domains := config.ServiceDomains()
	for name, port := range serviceDomainPorts(domains) {
		if domains[name] == u.Host && port > 0 {
			s.port = strconv.Itoa(port)
			s.domain = serviceDomainURLRe(u.Host)
			return s
		}
	}
	return nil
}

// bucketEnvKeys finds the service whose vars hold a URL ending in the bucket's
// placeholder, {{bucket}} or {{site}}, and the var set to that placeholder alone.
func bucketEnvKeys(services map[string]config.FrameworkServiceDef) (bucketKey, urlKey string) {
	for _, svc := range services {
		vars := map[string]string{}
		for _, kv := range svc.Vars {
			key, tmpl, _ := strings.Cut(kv, "=")
			vars[tmpl] = key
		}
		for tmpl, key := range vars {
			for _, ph := range []string{"{{bucket}}", "{{site}}"} {
				if strings.Contains(tmpl, "://") && strings.HasSuffix(tmpl, "/"+ph) && vars[ph] != "" {
					return vars[ph], key
				}
			}
		}
	}
	return "", ""
}

// rewrite points the file URLs of the storage's port at the tunnel, through the
// rewriter the LAN share uses. Presigned URLs keep their query, and the storage
// still verifies them: the proxy sends the host the signature covers.
func (s *shareStorage) rewrite(body []byte, tunnelHost string) []byte {
	if s == nil {
		return body
	}
	if s.domain != nil {
		body = s.domain.ReplaceAll(body, []byte("http:${1}${3}localhost:"+s.port+"${2}${4}"))
	}
	return rewriteLoopbackURLs(body, "https://"+tunnelHost, func(port string) bool { return port == s.port })
}

// withShareStorage serves the bucket's files read-only under the storage port's
// vitePrefix and refuses the rest of it: another port or bucket, a listing, a
// climb out of the bucket. Without a local bucket the prefix stays closed.
func withShareStorage(next http.Handler, s *shareStorage) http.Handler {
	if s == nil {
		return next
	}
	prefix := vitePrefix + s.port
	files := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "localhost:" + s.port
			req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
			req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, prefix)
			req.Host = s.host
			// the visitor's cookies and credentials stay behind
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
		},
		// uploads are untrusted: an HTML or SVG file must not run as the site
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Set("Content-Security-Policy", "sandbox")
			resp.Header.Set("X-Content-Type-Options", "nosniff")
			return nil
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, vitePrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key, ok := strings.CutPrefix(r.URL.Path, prefix+"/"+s.bucket+"/")
		if !ok || key == "" || strings.HasSuffix(key, "/") || slices.Contains(strings.Split(key, "/"), "..") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
