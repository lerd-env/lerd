package cli

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// storagePrefix is where a tunnel serves the site's RustFS bucket: the app's
// file URLs point at RustFS's local address, which a phone can't resolve and
// a public page may not load from loopback.
const storagePrefix = "/__lerd_s3__/"

// shareStorage is the bucket a site keeps on lerd's RustFS, as its .env names it.
type shareStorage struct {
	bucket string
	// origin is the scheme and host the app writes the bucket's file URLs
	// with (AWS_URL), which the proxy also fetches them from.
	origin *url.URL
}

// siteShareStorage reads the site's bucket from its .env. Nil when the site
// keeps no files on S3, or keeps them somewhere that is not local: a real bucket
// is already reachable from anywhere.
func siteShareStorage(sitePath string) *shareStorage {
	env := envfile.ReadValues(filepath.Join(sitePath, ".env"))
	bucket := env["AWS_BUCKET"]
	u, err := url.Parse(env["AWS_URL"])
	if bucket == "" || err != nil || u.Host == "" || !isLocalStorageHost(u.Hostname()) {
		return nil
	}
	if strings.Trim(u.Path, "/") != bucket {
		return nil // a virtual-host or prefixed layout this route doesn't map
	}
	return &shareStorage{bucket: bucket, origin: &url.URL{Scheme: u.Scheme, Host: u.Host}}
}

// isLocalStorageHost reports whether a file URL points at this machine: a
// loopback address or a domain under lerd's TLD.
func isLocalStorageHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.HasSuffix(host, "."+config.EffectiveTLD())
}

// rewrite points the bucket's file URLs at the tunnel. Presigned URLs keep
// their query, and RustFS still verifies them: the proxy sends the original
// host, which the signature covers.
func (s *shareStorage) rewrite(body []byte, tunnelHost string) []byte {
	if s == nil {
		return body
	}
	from := s.origin.String() + "/" + s.bucket + "/"
	to := "https://" + tunnelHost + storagePrefix + s.bucket + "/"
	body = bytes.ReplaceAll(body, []byte(from), []byte(to))
	return bytes.ReplaceAll(body, []byte(escapeSlashes(from)), []byte(escapeSlashes(to)))
}

// handler serves one file of the site's bucket, read-only. Anything else under
// the prefix is refused: another bucket, a listing (no key), or a path that
// climbs out of the bucket. The visitor's cookies and credentials stay behind.
func (s *shareStorage) handler() http.Handler {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = s.origin.Scheme
			req.URL.Host = s.origin.Host
			req.URL.Path = "/" + strings.TrimPrefix(req.URL.Path, storagePrefix)
			if req.URL.RawPath != "" {
				req.URL.RawPath = "/" + strings.TrimPrefix(req.URL.RawPath, storagePrefix)
			}
			req.Host = s.origin.Host
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
		},
	}
	if s.origin.Scheme == "https" {
		// a service domain carries lerd's local certificate
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		proxy.Transport = t
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key, ok := strings.CutPrefix(strings.TrimPrefix(r.URL.Path, storagePrefix), s.bucket+"/")
		if !ok || key == "" || strings.HasSuffix(key, "/") || slices.Contains(strings.Split(key, "/"), "..") {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

// withShareStorage routes the storage prefix to the site's bucket, and leaves
// every other request to next. Without a local bucket the prefix is just
// another path of the site.
func withShareStorage(next http.Handler, s *shareStorage) http.Handler {
	if s == nil {
		return next
	}
	files := s.handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, storagePrefix) {
			files.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
