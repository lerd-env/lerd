package certs

import (
	"bytes"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// caTrustPaths lists the aggregated system trust bundles searched for the
// mkcert root CA. These are the files mkcert's `-install` ultimately writes
// into (via update-ca-certificates / update-ca-trust / trust extract), so a
// hit here means the CA is already system-trusted and reinstalling would be a
// no-op that never prompts for sudo. Overridable in tests.
var caTrustPaths = []string{
	"/etc/ssl/certs/ca-certificates.crt", // Debian, Ubuntu, Arch, CachyOS
	"/etc/pki/tls/certs/ca-bundle.crt",   // Fedora, RHEL
	"/etc/ssl/cert.pem",                  // openSUSE and others
}

// platformTrustCheck reports whether der is trusted through a non-bundle store.
// The darwin build wires it to a keychain trust-settings check (mkcert installs
// its root there, not in a PEM bundle, and presence alone isn't trust — see
// ca_trust_darwin.go); nil on Linux, where caTrustPaths is authoritative.
var platformTrustCheck func(der []byte) bool

// platformPresenceCheck reports whether der merely exists in a platform
// certificate store, independent of any trust decision. The darwin build
// wires this to the same keychain dump platformTrustCheck used before it was
// upgraded to a real trust-settings check; nil on Linux, where a caTrustPaths
// hit already is the trust decision; there's no separate presence-only state
// to distinguish it from.
var platformPresenceCheck func(der []byte) bool

// caRootFunc resolves mkcert's CAROOT directory. Overridable in tests.
var caRootFunc = func() (string, error) {
	out, err := exec.Command(MkcertPath(), "-CAROOT").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CARoot returns mkcert's CAROOT directory, so a caller escalating to root can
// name the user's real CA location instead of assuming the default.
func CARoot() (string, error) { return caRootFunc() }

// rootCADER returns the DER bytes of mkcert's root CA certificate, or nil if
// CAROOT can't be resolved or rootCA.pem doesn't exist yet (no CA generated).
func rootCADER() []byte {
	root, err := caRootFunc()
	if err != nil || root == "" {
		return nil
	}
	return firstCertDER(readFileNoErr(filepath.Join(root, "rootCA.pem")))
}

// CATrusted reports whether mkcert's root CA is already trusted by the
// system: actually configured as a trusted root, not merely present in a
// keychain or store. Callers use it to skip the sudo announcement (and
// mkcert's chatty banner) on a reinstall where the CA is already trusted.
func CATrusted() bool {
	der := rootCADER()
	if der == nil {
		return false
	}
	for _, p := range caTrustPaths {
		if bundleContainsDER(readFileNoErr(p), der) {
			return true
		}
	}
	if platformTrustCheck != nil {
		return platformTrustCheck(der)
	}
	return false
}

// CAPresentButUntrusted reports the drifted state that let a reinstall
// silently skip re-establishing trust (see ca_trust_darwin.go): mkcert's
// root CA sits in a platform certificate store — so mkcert's own "already
// installed" self-check may treat it as already handled — but carries no
// actual trust decision. Always false when CATrusted() already reports true,
// or on a platform with no platformPresenceCheck wired (only darwin has a
// presence/trust gap to detect; a Linux bundle hit is the trust decision).
func CAPresentButUntrusted() bool {
	if platformPresenceCheck == nil || CATrusted() {
		return false
	}
	der := rootCADER()
	if der == nil {
		return false
	}
	return platformPresenceCheck(der)
}

func readFileNoErr(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}

// firstCertDER returns the DER bytes of the first CERTIFICATE block in pemBytes.
func firstCertDER(pemBytes []byte) []byte {
	for len(pemBytes) > 0 {
		block, rest := pem.Decode(pemBytes)
		if block == nil {
			return nil
		}
		if block.Type == "CERTIFICATE" {
			return block.Bytes
		}
		pemBytes = rest
	}
	return nil
}

// bundleContainsDER reports whether any CERTIFICATE block in pemBytes matches
// der. Comparing decoded DER makes the check robust to header comments and
// whitespace that trust extractors add around the same certificate.
func bundleContainsDER(pemBytes, der []byte) bool {
	for len(pemBytes) > 0 {
		block, rest := pem.Decode(pemBytes)
		if block == nil {
			return false
		}
		if block.Type == "CERTIFICATE" && bytes.Equal(block.Bytes, der) {
			return true
		}
		pemBytes = rest
	}
	return false
}
