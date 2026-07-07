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
// The darwin build wires it to a keychain lookup (mkcert installs its root
// there, not in a PEM bundle); nil on Linux, where caTrustPaths is authoritative.
var platformTrustCheck func(der []byte) bool

// caRootFunc resolves mkcert's CAROOT directory. Overridable in tests.
var caRootFunc = func() (string, error) {
	out, err := exec.Command(MkcertPath(), "-CAROOT").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CATrusted reports whether mkcert's root CA is already present in the system
// trust store. Callers use it to skip the sudo announcement (and mkcert's
// chatty banner) on a reinstall where the CA is already installed.
func CATrusted() bool {
	root, err := caRootFunc()
	if err != nil || root == "" {
		return false
	}
	der := firstCertDER(readFileNoErr(filepath.Join(root, "rootCA.pem")))
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
