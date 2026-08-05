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

	// A store that could not answer has not said the CA is untrusted. Reporting
	// it untrusted announces a sudo prompt and a full interactive mkcert
	// -install on every single install and update run.
	t.Run("platform trust state unreadable", func(t *testing.T) {
		origReadable := platformTrustReadable
		t.Cleanup(func() { platformTrustReadable = origReadable })
		caRootFunc = func() (string, error) { return caDir, nil }
		caTrustPaths = nil
		platformTrustCheck = func(der []byte) bool { return false }
		platformTrustReadable = func() bool { return false }
		if !CATrusted() {
			t.Fatal("an unreadable trust store must not be reported as untrusted")
		}

		platformTrustReadable = func() bool { return true }
		if CATrusted() {
			t.Fatal("a store that answered untrusted must be reported as untrusted")
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

// CAPresentButUntrusted is the signal ensureMkcertCA (darwin) uses to decide
// whether to repair trust directly instead of trusting mkcert -install's own
// (also fooled by the same drifted state) self-check to notice.
func TestCAPresentButUntrusted(t *testing.T) {
	origRoot, origPaths, origTrust, origPresence := caRootFunc, caTrustPaths, platformTrustCheck, platformPresenceCheck
	origReadable := platformTrustReadable
	t.Cleanup(func() {
		caRootFunc, caTrustPaths, platformTrustCheck, platformPresenceCheck = origRoot, origPaths, origTrust, origPresence
		platformTrustReadable = origReadable
	})
	caTrustPaths = nil // isolate from the bundle path; only the platform hooks matter here
	platformTrustReadable = func() bool { return true }

	caDir := t.TempDir()
	writeTestCA(t, caDir)
	caRootFunc = func() (string, error) { return caDir, nil }

	t.Run("present but not trusted", func(t *testing.T) {
		platformTrustCheck = func(der []byte) bool { return false }
		platformPresenceCheck = func(der []byte) bool { return true }
		if !CAPresentButUntrusted() {
			t.Fatal("expected the drifted present-but-untrusted state to be reported")
		}
	})

	t.Run("already trusted", func(t *testing.T) {
		platformTrustCheck = func(der []byte) bool { return true }
		platformPresenceCheck = func(der []byte) bool {
			t.Fatal("must not consult presence once CATrusted() already reports true")
			return false
		}
		if CAPresentButUntrusted() {
			t.Fatal("a genuinely trusted CA must never report as present-but-untrusted")
		}
	})

	t.Run("absent entirely", func(t *testing.T) {
		platformTrustCheck = func(der []byte) bool { return false }
		platformPresenceCheck = func(der []byte) bool { return false }
		if CAPresentButUntrusted() {
			t.Fatal("a CA missing from the keychain entirely is not the drifted state")
		}
	})

	t.Run("no platform presence hook wired", func(t *testing.T) {
		platformTrustCheck = func(der []byte) bool { return false }
		platformPresenceCheck = nil
		if CAPresentButUntrusted() {
			t.Fatal("must report false on a platform with no presence/trust distinction")
		}
	})

	// The repair prompts for an admin password, so it may only run on an
	// answered check. A store that could not be read reports nothing wrong,
	// otherwise a machine where the export keeps failing is asked to repair a
	// CA that was fine on every single install.
	t.Run("trust state unreadable", func(t *testing.T) {
		platformTrustCheck = func(der []byte) bool { return false }
		platformPresenceCheck = func(der []byte) bool { return true }
		platformTrustReadable = func() bool { return false }
		t.Cleanup(func() { platformTrustReadable = func() bool { return true } })
		if CAPresentButUntrusted() {
			t.Fatal("an unreadable trust store must not drive the privileged repair")
		}
	})
}
