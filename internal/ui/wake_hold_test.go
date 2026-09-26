package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	r := httptest.NewRequest(http.MethodGet, nginx.WakeHoldPath, nil)
	r.Header.Set("X-Lerd-Wake-Host", host)
	r.Header.Set("X-Lerd-Wake-Uri", uri)
	w := httptest.NewRecorder()
	withWakeHold(http.NotFoundHandler()).ServeHTTP(w, r)
	return w
}

func TestWakeHold_wakesTheSiteThenRedirectsBackToTheSamePath(t *testing.T) {
	pinged := stubWakeHold(t, 3)
	w := wakeRequest("shop.test", "/cart?item=4")
	if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "/cart?item=4" {
		t.Fatalf("got %d Location=%q, want 307 to /cart?item=4", w.Code, w.Header().Get("Location"))
	}
	if len(*pinged) != 1 || (*pinged)[0] != "shop" {
		t.Fatalf("pinged %v, want the site woken once", *pinged)
	}
}

func TestWakeHold_givesUpToTheWakingPage(t *testing.T) {
	stubWakeHold(t, 1<<30)
	wakeHoldMax = 5 * time.Millisecond
	if w := wakeRequest("shop.test", "/"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 so nginx serves the waking page", w.Code)
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
		if w := wakeRequest(tc.host, tc.uri); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want 404", tc.host, tc.uri, w.Code)
		}
	}
	if len(*pinged) != 0 {
		t.Fatalf("woke %v for a refused request", *pinged)
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
