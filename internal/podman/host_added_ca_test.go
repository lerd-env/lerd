package podman

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
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func testRootDER(t *testing.T, name string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func testRootPEM(t *testing.T, name string) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: testRootDER(t, name)})
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func withHostCASources(t *testing.T, sources []hostCASource) {
	t.Helper()
	old := hostAddedCASources
	t.Cleanup(func() { hostAddedCASources = old })
	hostAddedCASources = sources
}

// withTrustHostCA points config at a scratch dir with the opt-in set to on.
func withTrustHostCA(t *testing.T, on bool) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) { c.PHP.TrustHostCA = on }); err != nil {
		t.Fatal(err)
	}
}

// copiedRoots returns the subjects of the roots staged into the build context.
func copiedRoots(t *testing.T, tmp string) []string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(tmp, hostCAPrefix+"*.crt"))
	var names []string
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		block, _ := pem.Decode(data)
		if block == nil {
			t.Fatalf("%s is not PEM", m)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, cert.Subject.CommonName)
	}
	return names
}

// A root the host added is copied into the image and the trust store rebuilt,
// from the Debian folder (*.crt only, nested too) and an anchors folder (any
// file, PEM or DER), while lerd's own CA and files the host ignores stay out.
func TestHostAddedCABlock_CopiesHostRoots(t *testing.T) {
	debian := t.TempDir()
	anchors := t.TempDir()
	writeFile(t, filepath.Join(debian, "company.crt"), testRootPEM(t, "Company Root"))
	writeFile(t, filepath.Join(debian, "team", "team.crt"), testRootPEM(t, "Team Root"))
	writeFile(t, filepath.Join(debian, "ignored.pem"), testRootPEM(t, "Ignored By Debian"))
	writeFile(t, filepath.Join(debian, "lerd-mkcert-rootCA.crt"), testRootPEM(t, "lerd mkcert"))
	writeFile(t, filepath.Join(anchors, "proxy.der"), testRootDER(t, "Proxy Root"))
	writeFile(t, filepath.Join(anchors, "notes.txt"), []byte("not a certificate"))
	withHostCASources(t, []hostCASource{{dir: debian, crtOnly: true}, {dir: anchors}})
	withTrustHostCA(t, true)

	tmp := t.TempDir()
	got := hostAddedCABlock(tmp)

	want := "COPY " + hostCAPrefix + "*.crt /usr/local/share/ca-certificates/\nRUN update-ca-certificates\n"
	if !strings.Contains(got, want) {
		t.Errorf("block lacks %q:\n%s", want, got)
	}
	roots := strings.Join(copiedRoots(t, tmp), ",")
	for _, name := range []string{"Company Root", "Team Root", "Proxy Root"} {
		if !strings.Contains(roots, name) {
			t.Errorf("%s not copied, got %s", name, roots)
		}
	}
	for _, name := range []string{"Ignored By Debian", "lerd mkcert"} {
		if strings.Contains(roots, name) {
			t.Errorf("%s copied, got %s", name, roots)
		}
	}
}

// A bundle file holding several roots gets each one trusted.
func TestHostAddedCABlock_SplitsBundles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bundle.crt"), append(testRootPEM(t, "First"), testRootPEM(t, "Second")...))
	withHostCASources(t, []hostCASource{{dir: dir, crtOnly: true}})
	withTrustHostCA(t, true)

	tmp := t.TempDir()
	hostAddedCABlock(tmp)
	if got := len(copiedRoots(t, tmp)); got != 2 {
		t.Errorf("copied %d roots, want 2", got)
	}
}

// No folders, as on macOS, or folders with nothing usable add nothing, so the
// COPY glob never runs without a match.
func TestHostAddedCABlock_EmptyWithoutRoots(t *testing.T) {
	empty := t.TempDir()
	junk := t.TempDir()
	writeFile(t, filepath.Join(junk, "broken.crt"), []byte("-----BEGIN CERTIFICATE-----\nnope\n-----END CERTIFICATE-----\n"))
	withHostCASources(t, []hostCASource{{dir: filepath.Join(empty, "missing")}, {dir: empty}, {dir: junk, crtOnly: true}})
	withTrustHostCA(t, true)

	tmp := t.TempDir()
	if got := hostAddedCABlock(tmp); got != "" {
		t.Errorf("block = %q, want none", got)
	}
	if roots := copiedRoots(t, tmp); len(roots) != 0 {
		t.Errorf("staged %v, want none", roots)
	}
}

// Until the user opts in, roots the host added stay out of the image.
func TestHostAddedCABlock_OffByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "company.crt"), testRootPEM(t, "Company Root"))
	withHostCASources(t, []hostCASource{{dir: dir, crtOnly: true}})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tmp := t.TempDir()
	if got := hostAddedCABlock(tmp); got != "" {
		t.Errorf("block = %q, want none", got)
	}
	if roots := copiedRoots(t, tmp); len(roots) != 0 {
		t.Errorf("staged %v, want none", roots)
	}
}
