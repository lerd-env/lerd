package goserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// writeSiteCert puts a certificate where lerd keeps a secured site's own.
func writeSiteCert(t *testing.T, domain string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, domain+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(filepath.Join(dir, domain+".key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
}

func securedFixture(t *testing.T) *Handler {
	t.Helper()
	h, _ := fixture(t)
	sites := h.Sites()
	sites[0].Secured = true
	h.Sites = func() []config.Site { return sites }
	writeSiteCert(t, "app.test")
	return h
}

func tlsGet(t *testing.T, h *Handler, serverName, path string) (*http.Response, error) {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.TLS = &tls.Config{GetCertificate: h.GetCertificate}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: serverName}}}
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	req.Host = serverName
	return client.Do(req)
}

func TestTLS_securedSiteServesPHPOverHTTPS(t *testing.T) {
	h := securedFixture(t)
	resp, err := tlsGet(t, h, "app.test", "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "/index.php") {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
	if resp.TLS == nil || resp.TLS.PeerCertificates[0].Subject.CommonName != "app.test" {
		t.Error("not served the site's own certificate")
	}
}

// A worktree or worker subdomain rides the parent's wildcard certificate.
func TestTLS_subdomainUsesTheSiteCertificate(t *testing.T) {
	h := securedFixture(t)
	if _, err := tlsGet(t, h, "feat.app.test", "/"); err != nil {
		t.Fatal(err)
	}
}

func TestTLS_noCertificateForUnsecuredOrUnknownHosts(t *testing.T) {
	h, _ := fixture(t)
	for _, name := range []string{"app.test", "other.test"} {
		if _, err := tlsGet(t, h, name, "/"); err == nil {
			t.Errorf("%s: handshake succeeded without a secured site", name)
		}
	}
}

func TestHTTP_securedSiteRedirectsToHTTPS(t *testing.T) {
	h := securedFixture(t)
	rec := get(h, "GET", "http://app.test/x?y=1", "")
	if rec.Code != 302 || rec.Header().Get("Location") != "https://app.test/x?y=1" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// With nginx's HTTPS port moved, the redirect carries it, as the vhost does.
func TestHTTP_redirectCarriesAMovedHTTPSPort(t *testing.T) {
	h := securedFixture(t)
	cfgDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "lerd")
	os.MkdirAll(cfgDir, 0o755)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("nginx:\n    http_port: 10080\n    https_port: 10443\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := get(h, "GET", "http://app.test:10080/x", "")
	if loc := rec.Header().Get("Location"); loc != "https://app.test:10443/x" {
		t.Fatalf("Location %q", loc)
	}
}
