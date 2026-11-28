package nginx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

const containerForm = `server {
    listen 80;
    listen [::]:80;
    listen 443 ssl;
    listen [::]:443 ssl;
    listen 80 default_server;
    ssl_certificate /etc/nginx/certs/shop.test.crt;
    include /etc/nginx/custom.d/shop.test.conf;
    location ~ \.php$ {
        set $fpm "host.containers.internal";
    }
}
`

// The host's nginx reads the folders the container mounts at /etc/nginx, on
// the loopback ports the port forward relays 80 and 443 to, and reaches the
// host's own processes on loopback rather than through the VM gateway.
func TestHostForm_TranslatesTheContainersView(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got := string(hostForm([]byte(containerForm)))
	for _, want := range []string{
		"listen 127.0.0.1:17080;",
		"listen 127.0.0.1:17443 ssl;",
		"listen 127.0.0.1:17080 default_server;",
		"ssl_certificate " + filepath.Join(config.CertsDir(), "sites", "shop.test.crt") + ";",
		"include " + filepath.Join(config.NginxCustomD(), "shop.test.conf") + ";",
		`set $fpm "127.0.0.1";`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("host form missing %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"/etc/nginx/", "[::]", "host.containers.internal"} {
		if strings.Contains(got, gone) {
			t.Errorf("host form still carries %q:\n%s", gone, got)
		}
	}
	if again := string(hostForm([]byte(got))); again != got {
		t.Errorf("hostForm is not idempotent:\n%s", again)
	}
}

// A port at or above 1024 is not forwarded, so nginx listens on it directly.
func TestHostForm_ListensDirectlyOnAnUnforwardedPort(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.Nginx.HTTPPort, cfg.Nginx.HTTPSPort = 8080, 8443
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	got := string(hostForm([]byte("listen 80;\nlisten 443 ssl;\n")))
	if !strings.Contains(got, "listen 127.0.0.1:8080;") || !strings.Contains(got, "listen 127.0.0.1:8443 ssl;") {
		t.Errorf("unforwarded ports should be listened on directly:\n%s", got)
	}
}

// Behind the forward nginx answers on 17080 and 17443, and a PHP app that read
// that as its port would build every absolute URL with it.
func TestHostForm_PinsThePublicPortAndDropsContainerOnlyLines(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got := string(hostForm([]byte("user root;\npid /tmp/nginx.pid;\nproxy_set_header X-Forwarded-Port $server_port;\nfastcgi_param SERVER_PORT $server_port;\n")))
	for _, want := range []string{
		"proxy_set_header X-Forwarded-Port $lerd_public_port;",
		"fastcgi_param SERVER_PORT $lerd_public_port;",
		"pid " + filepath.Join(config.RunDir(), "nginx.pid") + ";",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("host form missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "user root;") || strings.Contains(got, "$server_port") {
		t.Errorf("host form kept a container-only line:\n%s", got)
	}
	if !strings.Contains(publicPortMap(), "17443 443;") || !strings.Contains(publicPortMap(), "${server_port}") {
		t.Errorf("public port map should map the loopback ports back, from ${server_port}:\n%s", publicPortMap())
	}
}

// The forward keeps 80 and 443 reachable from the LAN; a port nginx listens on
// itself follows LAN exposure, on every interface when it is on.
func TestHostForm_UnforwardedPortFollowsLANExposure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.Nginx.HTTPPort, cfg.Nginx.HTTPSPort = 8080, 8443
	cfg.LAN.Exposed = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	got := string(hostForm([]byte("listen 80;\nlisten 443 ssl;\n")))
	if !strings.Contains(got, "listen 8080;") || !strings.Contains(got, "listen 8443 ssl;") {
		t.Errorf("LAN-exposed unforwarded ports should listen on every interface:\n%s", got)
	}
	if again := string(hostForm([]byte(got))); again != got {
		t.Errorf("hostForm is not idempotent:\n%s", again)
	}
}
