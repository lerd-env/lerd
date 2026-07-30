//go:build darwin

package certs

import (
	"bytes"
	"crypto/sha1" // matching security's own export key, not a security boundary
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// keychainCertsPEM dumps every certificate in the keychain search list as
// concatenated PEM, regardless of trust settings. Used only to tell whether
// mkcert's CA is present at all (CAPresentButUntrusted); the actual trust
// decision comes from adminTrustSettingsPlist below — presence is not trust,
// see its doc comment. Overridable in tests.
var keychainCertsPEM = func() []byte {
	out, _ := exec.Command("security", "find-certificate", "-a", "-p").Output()
	return out
}

// adminTrustSettingsPlist exports the admin-domain trust settings — the
// domain mkcert -install writes into on macOS via a password-prompted
// `security add-trusted-cert -d` — as an XML plist. `security` can only
// write this export to a real file, not a pipe, so this shells through a
// temp file. Returns nil on any failure. Overridable in tests.
var adminTrustSettingsPlist = func() []byte {
	f, err := os.CreateTemp("", "lerd-trust-*.plist")
	if err != nil {
		return nil
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	if err := exec.Command("security", "trust-settings-export", "-d", path).Run(); err != nil {
		return nil
	}
	data, _ := os.ReadFile(path)
	return data
}

// fingerprintKeyPattern matches a trustList entry's key: security exports
// each certificate keyed by its uppercase-hex SHA-1 fingerprint.
var fingerprintKeyPattern = regexp.MustCompile(`<key>[0-9A-F]{40}</key>`)

// plistHasTrustEntry reports whether the admin trust-settings plist exported
// by `security trust-settings-export -d` has an entry for der's certificate
// that actually carries a trustSettings array.
//
// This is the check `security find-certificate` can't make, and the one
// that matters: a certificate can sit in the keychain — findable, matching
// this whole file's older keychainCertsPEM check — with its trust settings
// cleared independent of the certificate item itself (a macOS update is the
// most common real trigger). find-certificate reports both states
// identically as "present"; only the trust-settings export tells them apart,
// by whether the entry carries a trustSettings array at all. An entry
// without one is exactly the drifted state CAPresentButUntrusted exists to
// catch: present in the keychain, not actually trusted.
func plistHasTrustEntry(plist, der []byte) bool {
	sum := sha1.Sum(der)
	key := strings.ToUpper(hex.EncodeToString(sum[:]))
	marker := []byte("<key>" + key + "</key>")
	idx := bytes.Index(plist, marker)
	if idx < 0 {
		return false
	}
	// Scope the search to this entry's own block: from the marker to whichever
	// comes first, the next sibling entry (another fingerprint key) or the
	// trailing trustVersion key. A trustList entry never nests another
	// fingerprint key inside itself, so this bound is exact for this command's
	// schema without needing a full plist/XML parse.
	rest := plist[idx+len(marker):]
	end := len(rest)
	if i := nextSiblingKeyIndex(rest); i >= 0 && i < end {
		end = i
	}
	return bytes.Contains(rest[:end], []byte("<key>trustSettings</key>"))
}

// nextSiblingKeyIndex returns the start of whichever comes first in plist: the
// next fingerprint key or the trustVersion key, or -1 if neither is present.
func nextSiblingKeyIndex(plist []byte) int {
	best := -1
	if loc := fingerprintKeyPattern.FindIndex(plist); loc != nil {
		best = loc[0]
	}
	if i := bytes.Index(plist, []byte("<key>trustVersion</key>")); i >= 0 && (best < 0 || i < best) {
		best = i
	}
	return best
}

// systemKeychainPath is the keychain mkcert -install writes its root CA's
// admin-domain trust settings into on macOS, the write that requires the
// password prompt users see. Overridable in tests, though RepairSystemTrust
// itself is never exercised under test; see its doc comment.
var systemKeychainPath = "/Library/Keychains/System.keychain"

func init() {
	platformTrustCheck = func(der []byte) bool {
		return plistHasTrustEntry(adminTrustSettingsPlist(), der)
	}
	platformPresenceCheck = func(der []byte) bool {
		return bundleContainsDER(keychainCertsPEM(), der)
	}
}

// RepairSystemTrust re-asserts trust for mkcert's root CA directly, for the
// state CAPresentButUntrusted reports: the certificate sits in the keychain
// but carries no trust settings, most likely because a macOS update cleared
// them independent of the certificate item itself.
//
// This deliberately does not go through `mkcert -install`. mkcert's own
// "already installed" self-check (m.caCert.Verify against the system pool)
// is susceptible to the identical false positive plistHasTrustEntry exists
// to avoid: a self-signed root cryptographically verifies against itself
// regardless of its actual trust settings, so mkcert may silently skip
// re-establishing trust in exactly the state this function exists to fix.
// Running the same security add-trusted-cert command mkcert itself uses,
// directly, sidesteps that self-check entirely. The caller is expected to
// have already announced the sudo/password prompt this triggers.
func RepairSystemTrust() error {
	root, err := caRootFunc()
	if err != nil {
		return fmt.Errorf("resolving mkcert CAROOT: %w", err)
	}
	if root == "" {
		return fmt.Errorf("resolving mkcert CAROOT: empty path")
	}
	args := addTrustedCertArgs(systemKeychainPath, filepath.Join(root, "rootCA.pem"))
	cmd := exec.Command("security", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// addTrustedCertArgs builds the `security add-trusted-cert` argv that
// re-asserts admin-domain trust for rootCA at keychain: -d (admin store),
// -r trustRoot (the result mkcert itself sets), -k (destination keychain).
// Split out from RepairSystemTrust so the argument construction is testable
// without actually shelling out to a command that prompts for a password
// and writes to the real system keychain.
func addTrustedCertArgs(keychain, rootCA string) []string {
	return []string{"add-trusted-cert", "-d", "-r", "trustRoot", "-k", keychain, rootCA}
}
