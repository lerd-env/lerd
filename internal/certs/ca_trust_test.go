package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestCA generates a self-signed cert, writes its PEM to <dir>/rootCA.pem,
// and returns the PEM bytes so tests can plant it in (or omit it from) a bundle.
func writeTestCA(t *testing.T, dir string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mkcert test CA"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, "rootCA.pem"), certPEM, 0644); err != nil {
		t.Fatalf("write rootCA: %v", err)
	}
	return certPEM
}

func TestCATrusted(t *testing.T) {
	origRoot, origPaths, origPlat := caRootFunc, caTrustPaths, platformTrustCheck
	t.Cleanup(func() { caRootFunc, caTrustPaths, platformTrustCheck = origRoot, origPaths, origPlat })
	// Exercise the bundle logic in isolation; the keychain path has its own test.
	platformTrustCheck = nil

	caDir := t.TempDir()
	certPEM := writeTestCA(t, caDir)
	caRootFunc = func() (string, error) { return caDir, nil }

	bundleDir := t.TempDir()

	t.Run("present in bundle", func(t *testing.T) {
		bundle := filepath.Join(bundleDir, "present.crt")
		body := append([]byte("# some other anchor\n"), certPEM...)
		if err := os.WriteFile(bundle, body, 0644); err != nil {
			t.Fatal(err)
		}
		caTrustPaths = []string{bundle}
		if !CATrusted() {
			t.Fatal("expected CA to be reported as trusted")
		}
	})

	t.Run("absent from bundle", func(t *testing.T) {
		bundle := filepath.Join(bundleDir, "absent.crt")
		other := writeTestCA(t, t.TempDir()) // a different cert
		if err := os.WriteFile(bundle, other, 0644); err != nil {
			t.Fatal(err)
		}
		caTrustPaths = []string{bundle}
		if CATrusted() {
			t.Fatal("expected CA to be reported as not trusted")
		}
	})

	t.Run("no bundle files", func(t *testing.T) {
		caTrustPaths = []string{filepath.Join(bundleDir, "does-not-exist.crt")}
		if CATrusted() {
			t.Fatal("expected not trusted when no bundle is readable")
		}
	})

	t.Run("no rootCA", func(t *testing.T) {
		caRootFunc = func() (string, error) { return t.TempDir(), nil }
		caTrustPaths = []string{filepath.Join(bundleDir, "present.crt")}
		if CATrusted() {
			t.Fatal("expected not trusted when rootCA.pem is missing")
		}
	})
}
