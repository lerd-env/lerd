package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestGenerateWakingVhost_holdsInLerdUIWithWakingPageFallback(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "rr", Domains: []string{"rr.test"}, Path: "/srv/rr", HostPort: 5173}
	if err := GenerateWakingVhost(site); err != nil {
		t.Fatalf("GenerateWakingVhost: %v", err)
	}
	conf := readConf(t, filepath.Join(confD, "rr.test.conf"))
	for _, want := range []string{
		WakeHoldPath + ";",
		"access_log off;",
		"proxy_set_header X-Lerd-Wake-Uri $request_uri;",
		"proxy_set_header X-Lerd-Wake-Scheme $scheme;",
		"error_page 502 504 599 = @waking;",
		"try_files /waking.html =503",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("waking vhost lacks %q:\n%s", want, conf)
		}
	}
	if strings.Count(conf, "proxy_pass ") != 1 || strings.Contains(conf, ":5173") {
		t.Errorf("waking vhost must proxy only to the wake hold, never the stopped app:\n%s", conf)
	}
}

func TestGenerateWakingVhost_securedRemovesSeparateSSLConf(t *testing.T) {
	confD := setupConfD(t)
	if err := os.MkdirAll(confD, 0755); err != nil {
		t.Fatal(err)
	}
	// A secured host-proxy site's real backend lives in a separate -ssl.conf; the
	// waking swap must delete it so HTTPS stops routing to the dead dev server.
	sslPath := filepath.Join(confD, "rr.test-ssl.conf")
	if err := os.WriteFile(sslPath, []byte("server { listen 443 ssl; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Name: "rr", Domains: []string{"rr.test"}, Path: "/srv/rr", Secured: true}
	if err := GenerateWakingVhost(site); err != nil {
		t.Fatalf("GenerateWakingVhost: %v", err)
	}
	if _, err := os.Stat(sslPath); !os.IsNotExist(err) {
		t.Errorf("secured waking swap should remove %s", sslPath)
	}
	conf := readConf(t, filepath.Join(confD, "rr.test.conf"))
	if !strings.Contains(conf, "listen 443 ssl") || !strings.Contains(conf, "try_files /waking.html") {
		t.Errorf("secured waking vhost wrong:\n%s", conf)
	}
}

// TestGeneratePausedVhost_stillServesPausedPage guards the refactor: the paused
// vhost must keep serving paused.html after sharing code with the waking vhost.
func TestGeneratePausedVhost_stillServesPausedPage(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "app", Domains: []string{"app.test"}, Path: "/srv/app"}
	if err := GeneratePausedVhost(site); err != nil {
		t.Fatalf("GeneratePausedVhost: %v", err)
	}
	conf := readConf(t, filepath.Join(confD, "app.test.conf"))
	if !strings.Contains(conf, "try_files /paused.html =503") {
		t.Errorf("paused vhost should serve paused.html, got:\n%s", conf)
	}
}

// The held request must reach lerd-ui whole, method and body, since lerd-ui
// replays it to the app once the site is back.
func TestGenerateWakingVhost_forwardsTheRequestWhole(t *testing.T) {
	confD := setupConfD(t)
	if err := GenerateWakingVhost(config.Site{Name: "rr", Domains: []string{"rr.test"}, Path: "/srv/rr"}); err != nil {
		t.Fatal(err)
	}
	conf := readConf(t, filepath.Join(confD, "rr.test.conf"))
	for _, banned := range []string{"proxy_method", "proxy_pass_request_body off", "error_page 404", "error_page 500"} {
		if strings.Contains(conf, banned) {
			t.Errorf("waking vhost still has %q:\n%s", banned, conf)
		}
	}
}
