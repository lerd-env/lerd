//go:build darwin

package certs

import "testing"

// On macOS the trust check must consult the keychain, since mkcert never writes
// into the Linux PEM bundles caTrustPaths lists.
func TestCATrustedDarwinKeychain(t *testing.T) {
	origRoot, origPaths, origKC, origPlat := caRootFunc, caTrustPaths, keychainCertsPEM, platformTrustCheck
	t.Cleanup(func() {
		caRootFunc, caTrustPaths, keychainCertsPEM, platformTrustCheck = origRoot, origPaths, origKC, origPlat
	})
	platformTrustCheck = func(der []byte) bool { return bundleContainsDER(keychainCertsPEM(), der) }

	caDir := t.TempDir()
	certPEM := writeTestCA(t, caDir)
	caRootFunc = func() (string, error) { return caDir, nil }
	caTrustPaths = nil // no Linux bundles: the keychain is the only trust source

	t.Run("present in keychain", func(t *testing.T) {
		keychainCertsPEM = func() []byte { return append([]byte("# anchor\n"), certPEM...) }
		if !CATrusted() {
			t.Fatal("expected CA reported trusted when present in the keychain")
		}
	})

	t.Run("absent from keychain", func(t *testing.T) {
		other := writeTestCA(t, t.TempDir()) // a different CA
		keychainCertsPEM = func() []byte { return other }
		if CATrusted() {
			t.Fatal("expected CA reported not trusted when absent from the keychain")
		}
	})
}
