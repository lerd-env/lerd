package nginx

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// A restored vhost nginx has not reloaded yet still serves the hold, so a
// redirect then would bounce straight back; only a reload after it ends the wait.
func TestSiteWaking_waitsForTheReloadAfterTheRestore(t *testing.T) {
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

	write("location / { proxy_pass http://unix:x:"+WakeHoldPath+"; }", now)
	reloadAt(now)
	if !SiteWaking("shop.test") {
		t.Fatal("a waking vhost must hold")
	}
	write("server { root /srv/shop/public; }", now.Add(time.Second))
	if !SiteWaking("shop.test") {
		t.Fatal("restored but not reloaded yet must still hold")
	}
	reloadAt(now.Add(2 * time.Second))
	if SiteWaking("shop.test") {
		t.Fatal("restored and reloaded must release")
	}
}
