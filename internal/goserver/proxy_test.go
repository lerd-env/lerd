package goserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// proxyFixture serves one custom-container site whose container is an
// httptest server reporting the headers it was sent.
func proxyFixture(t *testing.T) *Handler {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s xfh=%s xfp=%s xff=%s port=%s uri=%s",
			r.Host, r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"),
			r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Forwarded-Port"), r.URL.RequestURI())
	}))
	t.Cleanup(upstream.Close)
	target, _ := url.Parse(upstream.URL)
	site := config.Site{Name: "app", Domains: []string{"app.test"}, Path: t.TempDir(), ContainerPort: 8080}
	return &Handler{
		Sites:    func() []config.Site { return []config.Site{site} },
		Upstream: func(config.Site) string { t.Fatal("a proxied site reached PHP-FPM"); return "" },
		Proxy: func(s config.Site) (*url.URL, bool) {
			return target, s.ContainerPort > 0
		},
	}
}

func TestProxy_sendsTheRequestOnLikeTheNginxVhost(t *testing.T) {
	h := proxyFixture(t)
	req := httptest.NewRequest("GET", "http://app.test/a/b?c=1", nil)
	req.RemoteAddr = "10.0.0.5:4242"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"host=app.test", "xfh=app.test", "xfp=http", "xff=10.0.0.5", "port=80", "uri=/a/b?c=1"} {
		if !strings.Contains(body, want) {
			t.Errorf("upstream saw %q, want %q", body, want)
		}
	}
}

// Behind a share tunnel the public host and scheme arrive as X-Forwarded-*,
// and the app has to build its URLs from those, not from the .test name.
func TestProxy_keepsTheHostAndSchemeAShareForwarded(t *testing.T) {
	h := proxyFixture(t)
	req := httptest.NewRequest("GET", "http://app.test/", nil)
	req.Header.Set("X-Forwarded-Host", "abc.share.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"host=abc.share.example", "xfh=abc.share.example", "xfp=https"} {
		if !strings.Contains(body, want) {
			t.Errorf("upstream saw %q, want %q", body, want)
		}
	}
}

func TestProxy_deadUpstreamIs502(t *testing.T) {
	h := proxyFixture(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead, _ := url.Parse("http://" + ln.Addr().String())
	ln.Close()
	h.Proxy = func(config.Site) (*url.URL, bool) { return dead, true }
	if rec := get(h, "GET", "http://app.test/", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
}

func TestSiteTarget_pointsEachKindWhereNginxDoes(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cases := []struct {
		name string
		site config.Site
		want string
	}{
		{"custom container", config.Site{Name: "c", ContainerPort: 8080}, "http://lerd-custom-c:8080"},
		{"custom container with tls", config.Site{Name: "c", ContainerPort: 8443, ContainerSSL: true}, "https://lerd-custom-c:8443"},
		{"frankenphp", config.Site{Name: "f", Runtime: "frankenphp"}, "http://lerd-fp-f:8000"},
		{"host proxy", config.Site{Name: "h", HostPort: 5173}, "http://host.containers.internal:5173"},
		{"host proxy with tls", config.Site{Name: "h", HostPort: 5173, HostSSL: true}, "https://host.containers.internal:5173"},
		{"php-fpm", config.Site{Name: "p", PHPVersion: "8.4"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, ok := SiteTarget(c.site)
			got := ""
			if ok {
				got = u.String()
			}
			if got != c.want {
				t.Errorf("SiteTarget = %q, want %q", got, c.want)
			}
		})
	}
}
