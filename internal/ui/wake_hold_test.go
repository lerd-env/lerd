package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

func stubWakeHold(t *testing.T, asleepPolls int) (pinged *[]string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.AddSite(config.Site{Name: "shop", Path: "/srv/shop", Domains: []string{"shop.test"}}); err != nil {
		t.Fatal(err)
	}
	prevPing, prevAsleep, prevPoll, prevMax, prevSettle := wakeHoldPing, wakeHoldAsleep, wakeHoldPoll, wakeHoldMax, wakeHoldSettle
	t.Cleanup(func() {
		wakeHoldPing, wakeHoldAsleep, wakeHoldPoll, wakeHoldMax, wakeHoldSettle = prevPing, prevAsleep, prevPoll, prevMax, prevSettle
	})
	pinged = &[]string{}
	wakeHoldPing = func(s string) { *pinged = append(*pinged, s) }
	wakeHoldAsleep = func(domain string) bool {
		if domain != "shop.test" {
			t.Errorf("waited on %q", domain)
		}
		asleepPolls--
		return asleepPolls >= 0
	}
	wakeHoldPoll, wakeHoldSettle = time.Millisecond, 0
	return pinged
}

func wakeRequest(host, uri string) *httptest.ResponseRecorder {
	return wakeRequestWith(httptest.NewRequest(http.MethodGet, nginx.WakeHoldPath, nil), host, uri)
}

func wakeRequestWith(r *http.Request, host, uri string) *httptest.ResponseRecorder {
	r.Header.Set("X-Lerd-Wake-Host", host)
	r.Header.Set("X-Lerd-Wake-Uri", uri)
	r.Header.Set("X-Lerd-Wake-Scheme", "http")
	w := httptest.NewRecorder()
	withWakeHold(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

// fakeNginx stands in for nginx serving the restored site and records what the
// hold replayed to it.
func fakeNginx(t *testing.T, status int, body string) *http.Request {
	t.Helper()
	var got http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = *r
		got.Body = io.NopCloser(strings.NewReader(string(b)))
		w.Header().Set("X-App", "yes")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prev := wakeHoldAddr
	t.Cleanup(func() { wakeHoldAddr = prev })
	wakeHoldAddr = func(string) string { return strings.TrimPrefix(srv.URL, "http://") }
	return &got
}

// A webhook or API client gets the app's own response to its own request, not
// a redirect it may not follow: method, path, body and host all reach the app.
func TestWakeHold_replaysTheRequestToTheWokenSite(t *testing.T) {
	pinged := stubWakeHold(t, 3)
	got := fakeNginx(t, http.StatusCreated, `{"ok":true}`)

	r := httptest.NewRequest(http.MethodPost, nginx.WakeHoldPath, strings.NewReader(`{"event":"paid"}`))
	r.Header.Set("Content-Type", "application/json")
	w := wakeRequestWith(r, "shop.test", "/webhooks/stripe?x=1")

	if w.Code != http.StatusCreated || w.Body.String() != `{"ok":true}` || w.Header().Get("X-App") != "yes" {
		t.Fatalf("got %d %q, want the app's own 201 and body", w.Code, w.Body.String())
	}
	body, _ := io.ReadAll(got.Body)
	if got.Method != http.MethodPost || got.URL.Path != "/webhooks/stripe" || got.URL.RawQuery != "x=1" || got.Host != "shop.test" || string(body) != `{"event":"paid"}` {
		t.Fatalf("app saw %s %s?%s host=%s body=%q", got.Method, got.URL.Path, got.URL.RawQuery, got.Host, body)
	}
	if got.Header.Get("X-Lerd-Wake-Uri") != "" || got.Header.Get(wakeHoldReplayed) == "" {
		t.Fatal("wake headers leaked to the app, or the replay was not marked")
	}
	if len(*pinged) != 1 || (*pinged)[0] != "shop" {
		t.Fatalf("pinged %v", *pinged)
	}
}

// The app's own errors are its answer; only the hold's failures (599) become
// the waking page.
func TestWakeHold_passesTheAppsErrorsThrough(t *testing.T) {
	stubWakeHold(t, 0)
	fakeNginx(t, http.StatusNotFound, "no such page")
	if w := wakeRequest("shop.test", "/missing"); w.Code != http.StatusNotFound || w.Body.String() != "no such page" {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestWakeHold_redirectsAWebsocketUpgrade(t *testing.T) {
	stubWakeHold(t, 0)
	r := httptest.NewRequest(http.MethodGet, nginx.WakeHoldPath, nil)
	r.Header.Set("Upgrade", "websocket")
	w := wakeRequestWith(r, "shop.test", "/ws")
	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/ws" {
		t.Fatalf("got %d Location=%q", w.Code, w.Header().Get("Location"))
	}
}

func TestWakeHold_givesUpToTheWakingPage(t *testing.T) {
	stubWakeHold(t, 1<<30)
	wakeHoldMax = 5 * time.Millisecond
	if w := wakeRequest("shop.test", "/"); w.Code != wakeHoldFailed {
		t.Fatalf("got %d, want %d so nginx serves the waking page", w.Code, wakeHoldFailed)
	}
}

func TestWakeHold_refusesAnythingButARelativePathOnAKnownSite(t *testing.T) {
	pinged := stubWakeHold(t, 0)
	for _, tc := range []struct{ host, uri string }{
		{"shop.test", "//evil.example/"},
		{"shop.test", "https://evil.example/"},
		{"other.test", "/"},
		{"", "/"},
	} {
		if w := wakeRequest(tc.host, tc.uri); w.Code != wakeHoldFailed {
			t.Errorf("%s %s: got %d, want %d", tc.host, tc.uri, w.Code, wakeHoldFailed)
		}
	}
	if len(*pinged) != 0 {
		t.Fatalf("woke %v for a refused request", *pinged)
	}
}

// A replay landing back in the hold (nginx not serving the restored vhost yet)
// falls back to the waking page instead of replaying forever.
func TestWakeHold_neverReplaysAReplay(t *testing.T) {
	stubWakeHold(t, 0)
	r := httptest.NewRequest(http.MethodGet, nginx.WakeHoldPath, nil)
	r.Header.Set(wakeHoldReplayed, "1")
	if w := wakeRequestWith(r, "shop.test", "/"); w.Code != wakeHoldFailed {
		t.Fatalf("got %d, want %d", w.Code, wakeHoldFailed)
	}
}

// A restored vhost nginx has not reloaded yet still serves the hold, so a
// redirect then would bounce straight back; only a reload after it ends the wait.
func TestSiteVhostWaking_waitsForTheReloadAfterTheRestore(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	confD := config.NginxConfD()
	if err := os.MkdirAll(confD, 0755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(confD, "shop.test.conf")
	write := func(body string, at time.Time) {
		if err := os.WriteFile(conf, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(conf, at, at)
	}
	marker := filepath.Join(config.RunDir(), "nginx-reloaded")
	reloadAt := func(at time.Time) {
		_ = os.MkdirAll(config.RunDir(), 0755)
		_ = os.WriteFile(marker, nil, 0644)
		_ = os.Chtimes(marker, at, at)
	}
	now := time.Now()

	write("location / { proxy_pass http://unix:x:"+nginx.WakeHoldPath+"; }", now)
	reloadAt(now)
	if !siteVhostWaking("shop.test") {
		t.Fatal("a waking vhost must hold")
	}
	write("server { root /srv/shop/public; }", now.Add(time.Second))
	if !siteVhostWaking("shop.test") {
		t.Fatal("restored but not reloaded yet must still hold")
	}
	reloadAt(now.Add(2 * time.Second))
	if siteVhostWaking("shop.test") {
		t.Fatal("restored and reloaded must release")
	}
}
