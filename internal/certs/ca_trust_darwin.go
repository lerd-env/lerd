//go:build darwin

package certs

import "os/exec"

// keychainCertsPEM dumps every certificate in the keychain search list as
// concatenated PEM. Overridable in tests.
var keychainCertsPEM = func() []byte {
	out, _ := exec.Command("security", "find-certificate", "-a", "-p").Output()
	return out
}

// mkcert installs its root into the keychain, not the PEM bundles caTrustPaths
// scans, so on macOS the trust check runs against the keychain dump. The same
// DER-equality match as the bundle path handles the headers `security` prints.
func init() {
	platformTrustCheck = func(der []byte) bool {
		return bundleContainsDER(keychainCertsPEM(), der)
	}
}
