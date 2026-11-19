//go:build conformance

package webserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// TestConformance serves sites through the current backend in real containers
// and checks what a browser gets. It provisions containers, so build it with
// -tags conformance and run it in a test guest, never on the host.
func TestConformance(t *testing.T) {
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("podman not available")
	}
	tmp := t.TempDir()
	// Podman keeps its storage under XDG_DATA_HOME, so it must see the real
	// one; only lerd's own paths move into tmp.
	podmanEnv = os.Environ()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	site := writeSite(t, filepath.Join(tmp, "site"))
	sites := map[string]config.Site{
		"plain":  {Name: "plain", Domains: []string{"plain.test"}, Path: site, PHPVersion: "8.4"},
		"secure": {Name: "secure", Domains: []string{"secure.test"}, Path: site, PHPVersion: "8.4", Secured: true},
		"custom": {Name: "custom", Domains: []string{"custom.test"}, Path: site, ContainerPort: 80},
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(EnsureConfig())
	must(EnsureDefaultSite())
	for _, s := range sites {
		if s.Secured {
			writeCert(t, s.PrimaryDomain())
		}
		must(config.AddSite(s))
		must(ApplySite(s))
	}
	must(os.MkdirAll(config.PausedDir(), 0o755))
	must(os.WriteFile(filepath.Join(config.PausedDir(), "paused.html"), []byte("conformance paused page"), 0o644))
	paused := config.Site{Name: "paused", Domains: []string{"paused.test"}, Path: site, Paused: true}
	must(config.AddSite(paused))
	must(ApplyPausedSite(paused))

	h := startHarness(t, tmp, site)

	cases := []struct {
		name, method, scheme, host, path, body string
		status                                 int
		want, location                         string
		goReady                                bool
	}{
		{goReady: true, name: "php over http", method: "GET", scheme: "http", host: "plain.test", path: "/", status: 200, want: "conformance GET fpm-fcgi"},
		{goReady: true, name: "post body reaches php", method: "POST", scheme: "http", host: "plain.test", path: "/", body: "hello", status: 200, want: "conformance POST fpm-fcgi hello"},
		{goReady: true, name: "front controller", method: "GET", scheme: "http", host: "plain.test", path: "/some/route", status: 200, want: "conformance GET fpm-fcgi"},
		{goReady: true, name: "php over https", method: "GET", scheme: "https", host: "secure.test", path: "/", status: 200, want: "conformance GET fpm-fcgi"},
		{goReady: true, name: "secured redirects http", method: "GET", scheme: "http", host: "secure.test", path: "/x?y=1", status: 302, location: "https://secure.test/x?y=1"},
		{name: "custom container proxied", method: "GET", scheme: "http", host: "custom.test", path: "/", status: 200, want: "Welcome to nginx"},
		{name: "paused page", method: "GET", scheme: "http", host: "paused.test", path: "/", want: "conformance paused page"},
		{goReady: true, name: "unknown host", method: "GET", scheme: "http", host: "nobody.test", path: "/", status: 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if h.goServer && !c.goReady {
				t.Skip("not served by the Go web server yet")
			}
			status, body, location := h.do(t, c.method, c.scheme, c.host, c.path, c.body)
			if c.status != 0 && status != c.status {
				t.Errorf("status %d, want %d; body %.200q", status, c.status, body)
			}
			if c.want != "" && !strings.Contains(body, c.want) {
				t.Errorf("body %.200q, want it to contain %q", body, c.want)
			}
			if c.location != "" && location != c.location {
				t.Errorf("Location %q, want %q", location, c.location)
			}
		})
	}
}

func writeSite(t *testing.T, dir string) string {
	t.Helper()
	pub := filepath.Join(dir, "public")
	if err := os.MkdirAll(pub, 0o755); err != nil {
		t.Fatal(err)
	}
	php := `<?php echo "conformance ", $_SERVER["REQUEST_METHOD"], " ", PHP_SAPI, " ", file_get_contents("php://input");`
	if err := os.WriteFile(filepath.Join(pub, "index.php"), []byte(php), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeCert drops a self-signed certificate where the vhost expects the
// site's own, since the harness checks serving, not trust.
func writeCert(t *testing.T, domain string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain, "*." + domain},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	dir := filepath.Join(config.CertsDir(), "sites")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, typ string, b []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(domain+".crt", "CERTIFICATE", der)
	write(domain+".key", "EC PRIVATE KEY", kb)
}

type harness struct {
	http, https string
	goServer    bool
}

var podmanEnv []string

func podmanCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("podman", args...)
	cmd.Env = podmanEnv
	return cmd
}

// startHarness runs PHP-FPM, a custom-container upstream and the web server on
// a network of their own, so the guest's real lerd containers are untouched.
func startHarness(t *testing.T, tmp, site string) harness {
	t.Helper()
	id := fmt.Sprintf("lerdconf%d", os.Getpid())
	podman := func(args ...string) string {
		t.Helper()
		out, err := podmanCmd(args...).CombinedOutput()
		if err != nil {
			t.Fatalf("podman %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	podman("network", "create", id)
	t.Cleanup(func() {
		podmanCmd("rm", "-f", "-t", "0", id+"-nginx", id+"-fpm", id+"-custom").Run()
		podmanCmd("network", "rm", "-f", id).Run()
	})
	gw := podman("network", "inspect", "-f", "{{range .Subnets}}{{.Gateway}}{{end}}", id)

	// The rendered resolver names lerd's own network; point it at this one.
	conf := filepath.Join(config.NginxDir(), "nginx.conf")
	data, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	data = regexp.MustCompile(`resolver \S+`).ReplaceAll(data, []byte("resolver "+gw))
	if err := os.WriteFile(conf, data, 0o644); err != nil {
		t.Fatal(err)
	}

	noLabel := "--security-opt=label=disable"
	podman("run", "-d", "--name", id+"-fpm", "--network", id, "--network-alias", "lerd-php84-fpm", noLabel,
		"-v", site+":"+site+":ro", "localhost/lerd-php84-fpm:local", "php-fpm", "-F", "-R")
	podman("run", "-d", "--name", id+"-custom", "--network", id, "--network-alias", "lerd-custom-custom", noLabel,
		"docker.io/library/nginx:alpine")
	// LERD_CONFORMANCE_BACKEND=go runs lerd's own server, from the static
	// binary in LERD_BIN, where nginx would otherwise be.
	goServer := os.Getenv("LERD_CONFORMANCE_BACKEND") == "go"
	if goServer {
		bin := os.Getenv("LERD_BIN")
		if bin == "" {
			t.Fatal("LERD_CONFORMANCE_BACKEND=go needs LERD_BIN, a static lerd binary")
		}
		podman("run", "-d", "--name", id+"-nginx", "--network", id, noLabel,
			"-p", "127.0.0.1::80", "-p", "127.0.0.1::443",
			"-e", "XDG_DATA_HOME="+tmp, "-e", "XDG_CONFIG_HOME="+tmp,
			"-v", bin+":/usr/local/bin/lerd:ro", "-v", tmp+":"+tmp+":ro",
			"--entrypoint", "/usr/local/bin/lerd", "docker.io/library/nginx:alpine", "web-serve")
	} else {
		nginxDir := config.NginxDir()
		podman("run", "-d", "--name", id+"-nginx", "--network", id, noLabel,
			"-p", "127.0.0.1::80", "-p", "127.0.0.1::443",
			"-v", conf+":/etc/nginx/nginx.conf:ro",
			"-v", filepath.Join(nginxDir, "conf.d")+":/etc/nginx/conf.d:ro",
			"-v", filepath.Join(nginxDir, "custom.d")+":/etc/nginx/custom.d:ro",
			"-v", filepath.Join(nginxDir, "http.d")+":/etc/nginx/http.d:ro",
			"-v", filepath.Join(config.CertsDir(), "sites")+":/etc/nginx/certs:ro",
			"-v", tmp+":"+tmp+":ro",
			"docker.io/library/nginx:alpine")
	}
	port := func(p string) string {
		out := podman("port", id+"-nginx", p)
		_, port, _ := net.SplitHostPort(strings.Fields(out)[0])
		return port
	}
	h := harness{http: port("80"), https: port("443"), goServer: goServer}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if c, err := net.DialTimeout("tcp", "127.0.0.1:"+h.http, time.Second); err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			logs, _ := podmanCmd("logs", id+"-nginx").CombinedOutput()
			t.Fatalf("web server never listened:\n%s", logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return h
}

// do sends one request as a browser on this machine would, with the Host the
// site answers to, and returns status, body and redirect target.
func (h harness) do(t *testing.T, method, scheme, host, path, body string) (int, string, string) {
	t.Helper()
	port := h.http
	if scheme == "https" {
		port = h.https
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+port)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: host},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// A container that has only just started can refuse or 502 for a moment;
	// anything still failing after the deadline is a real failure.
	var lastErr error
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		req, err := http.NewRequest(method, scheme+"://"+host+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusBadGateway {
			lastErr = fmt.Errorf("502: %.200s", b)
			continue
		}
		return resp.StatusCode, string(b), resp.Header.Get("Location")
	}
	t.Fatalf("%s %s://%s%s: %v", method, scheme, host, path, lastErr)
	return 0, "", ""
}
