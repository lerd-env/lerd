package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestGenerateWakingVhost_servesWakingPageNotProxy(t *testing.T) {
	confD := setupConfD(t)
	site := config.Site{Name: "rr", Domains: []string{"rr.test"}, Path: "/srv/rr"}
	if err := GenerateWakingVhost(site); err != nil {
		t.Fatalf("GenerateWakingVhost: %v", err)
	}
	conf := readConf(t, filepath.Join(confD, "rr.test.conf"))
	if !strings.Contains(conf, "try_files /waking.html =503") {
		t.Errorf("waking vhost should serve waking.html, got:\n%s", conf)
	}
	if strings.Contains(conf, "proxy_pass") {
		t.Errorf("waking vhost must not proxy to the stopped dev server, got:\n%s", conf)
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
