package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The upstream signs against the Host it is handed, so the proxy has to forward
// the name the client signed for. Forwarding the container's would turn every
// presigned URL into a signature mismatch.
func TestGenerateServiceProxyVhost_ForwardsTheRequestedHostOverTLS(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := GenerateServiceProxyVhost("rustfs.test", "lerd-rustfs", 9000, true); err != nil {
		t.Fatalf("GenerateServiceProxyVhost: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(config.NginxConfD(), "rustfs.test.conf"))
	if err != nil {
		t.Fatalf("reading vhost: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		"server_name rustfs.test;",
		"ssl_certificate /etc/nginx/certs/rustfs.test.crt;",
		`set $backend "lerd-rustfs";`,
		"proxy_pass http://$backend:9000;",
		"proxy_set_header Host $host;",
		"return 301 https://$host$request_uri;",
		"client_max_body_size 0;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("vhost missing %q:\n%s", want, got)
		}
	}
}

func TestGenerateServiceProxyVhost_PlainHTTPWhenNotSecured(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := GenerateServiceProxyVhost("rustfs.test", "lerd-rustfs", 9000, false); err != nil {
		t.Fatalf("GenerateServiceProxyVhost: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(config.NginxConfD(), "rustfs.test.conf"))
	if err != nil {
		t.Fatalf("reading vhost: %v", err)
	}
	if strings.Contains(string(body), "ssl_certificate") {
		t.Errorf("plain vhost should carry no TLS directives:\n%s", body)
	}
}

// A service domain belongs to no site, so the orphan rule in the repair sweep
// would delete the vhost that makes it answer at all.
func TestRepairVhosts_KeepsAServiceDomainVhost(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]config.ServiceConfig{}
	}
	sc := cfg.Services["rustfs"]
	sc.Domain = "rustfs.test"
	cfg.Services["rustfs"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	if err := GenerateServiceProxyVhost("rustfs.test", "lerd-rustfs", 9000, true); err != nil {
		t.Fatalf("GenerateServiceProxyVhost: %v", err)
	}
	conf := filepath.Join(config.NginxConfD(), "rustfs.test.conf")

	RepairVhosts()

	if _, err := os.Stat(conf); err != nil {
		t.Fatalf("the service vhost was removed by the repair sweep: %v", err)
	}
}
