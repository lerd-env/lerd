package goserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// paramsFixture's PHP-FPM stand-in reports the variables nginx's PHP location
// sets beyond the CGI basics. Go's responder turns HTTP_* back into headers.
func paramsFixture(t *testing.T) *Handler {
	t.Helper()
	h, _ := fixture(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := fcgi.ProcessEnv(r)
		fmt.Fprintf(w, "host=%s|server=%s|xfp=%s|xfport=%s|xff=%s|real=%s|cookie=%s|site=%s|sent=%s",
			r.Host, env["SERVER_NAME"], r.Header.Get("X-Forwarded-Proto"), r.Header.Get("X-Forwarded-Port"),
			r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-Ip"), r.Header.Get("Cookie"), env["LERD_SITE"], env["LERD_NGINX_SENT"])
	}))
	h.Upstream = func(config.Site) string { return ln.Addr().String() }
	return h
}

func field(body, name string) string {
	for _, part := range strings.Split(body, "|") {
		if v, ok := strings.CutPrefix(part, name+"="); ok {
			return v
		}
	}
	return "<missing>"
}

func TestParams_directRequestCarriesTheSPXKeyAndSite(t *testing.T) {
	h := paramsFixture(t)
	key, err := config.LoadOrGenerateProfilerKey()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "http://app.test/", nil)
	req.RemoteAddr = "10.0.0.3:5000"
	req.Header.Set("Cookie", "laravel_session=abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b := rec.Body.String()
	checks := map[string]string{
		"host": "app.test", "server": "app.test", "xfp": "http", "xfport": "80",
		"xff": "10.0.0.3", "real": "10.0.0.3", "site": "app",
		"cookie": "laravel_session=abc; SPX_KEY=" + key,
	}
	for k, want := range checks {
		if got := field(b, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if sent, err := strconv.ParseFloat(field(b, "sent"), 64); err != nil || sent < 1e9 {
		t.Errorf("LERD_NGINX_SENT %q is not the time the request was handed over", field(b, "sent"))
	}
}

// Behind a share, PHP sees the public host and scheme, and gets no SPX key: a
// visitor on the internet must not be able to switch the profiler on.
func TestParams_sharedRequestSeesThePublicHostAndNoSPXKey(t *testing.T) {
	h := paramsFixture(t)
	req := httptest.NewRequest("GET", "http://app.test/", nil)
	req.Header.Set("X-Forwarded-Host", "abc.share.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Port", "443")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b := rec.Body.String()
	for k, want := range map[string]string{"host": "abc.share.example", "server": "abc.share.example", "xfp": "https", "xfport": "443", "cookie": "; SPX_KEY="} {
		if got := field(b, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestParams_profilingOnAddsSPXEnabled(t *testing.T) {
	h := paramsFixture(t)
	cfgDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "lerd")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("profiler:\n    enabled: true\n"), 0o644)
	cookie := field(get(h, "GET", "http://app.test/", "").Body.String(), "cookie")
	if !strings.HasSuffix(cookie, "; SPX_ENABLED=1") {
		t.Errorf("cookie %q, want SPX_ENABLED=1 while profiling", cookie)
	}
}

func TestProfiler_stateEndpointFollowsTheConfig(t *testing.T) {
	h := paramsFixture(t)
	if got := get(h, "GET", "http://profiler.localhost/_lerd/profiler-state", "").Body.String(); got != "off" {
		t.Fatalf("state %q, want off", got)
	}
	cfgDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "lerd")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("profiler:\n    enabled: true\n"), 0o644)
	if got := get(h, "GET", "http://profiler.localhost/_lerd/profiler-state", "").Body.String(); got != "on" {
		t.Fatalf("state %q, want on", got)
	}
}

// The SPX UI is served by the default PHP version through lerd's bridge
// script, with the key SPX checks.
func TestProfiler_uiRunsTheBridgeWithTheKey(t *testing.T) {
	h, _ := fixture(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s", fcgi.ProcessEnv(r)["SCRIPT_FILENAME"], r.Header.Get("Cookie"))
	}))
	h.ProfilerUpstream = func() (string, string) { return ln.Addr().String(), "/usr/local/etc/lerd/dump-bridge.php" }
	key, _ := config.LoadOrGenerateProfilerKey()
	got := get(h, "GET", "http://profiler.localhost/?SPX_UI_URI=/", "").Body.String()
	if got != "/usr/local/etc/lerd/dump-bridge.php|SPX_KEY="+key {
		t.Fatalf("got %q", got)
	}
}
