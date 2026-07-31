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
// decision comes from adminTrustSettingsPlist below, since a certificate can
// outlive the trust settings that made it trusted. Overridable in tests.
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

// trustResultPattern matches an entry's kSecTrustSettingsResult values.
var trustResultPattern = regexp.MustCompile(`<key>kSecTrustSettingsResult</key>\s*<integer>(\d+)</integer>`)

// trustSettingsExported reports whether plist is an admin trust-settings
// export that actually parsed, as opposed to nothing at all. A `security`
// that failed, or that one day writes something other than this XML, must
// read as "no answer" rather than as "not trusted": drawing a conclusion
// from a read that did not happen is what makes an install re-run a
// privileged repair on every run (the shape of issue #1101).
func trustSettingsExported(plist []byte) bool {
	return bytes.Contains(plist, []byte("<key>trustList</key>"))
}

// plistTrustsCert reports whether the admin trust-settings plist exported by
// `security trust-settings-export -d` trusts der's certificate.
//
// Presence of the entry is the trust decision. Per SecTrustSettings, an
// absent or empty trust-settings array means "trust this certificate as a
// root", which is how macOS records everything `security add-trusted-cert`
// installs; mkcert is unusual in writing an explicit array of its own
// through trust-settings-import afterwards. So only an entry whose explicit
// settings all refuse (deny, or unspecified, which defers to a system trust
// this certificate does not have) counts as untrusted, and a certificate
// with no entry at all is the drifted state CAPresentButUntrusted catches:
// still in the keychain, no longer trusted by it.
func plistTrustsCert(plist, der []byte) bool {
	block, ok := trustEntryBlock(plist, der)
	if !ok {
		return false
	}
	results := trustResultPattern.FindAllSubmatch(block, -1)
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		// kSecTrustSettingsResultTrustRoot / TrustAsRoot.
		if v := string(r[1]); v == "1" || v == "2" {
			return true
		}
	}
	return false
}

// trustEntryBlock returns der's own entry in the export, bounded so a
// neighbour's settings can never be read as this certificate's. The block
// runs from the fingerprint key to whichever comes first, the next sibling
// entry or the trailing trustVersion key: a trustList entry never nests
// another fingerprint key, so the bound is exact for this schema without a
// full XML parse.
func trustEntryBlock(plist, der []byte) ([]byte, bool) {
	sum := sha1.Sum(der)
	key := strings.ToUpper(hex.EncodeToString(sum[:]))
	marker := []byte("<key>" + key + "</key>")
	idx := bytes.Index(plist, marker)
	if idx < 0 {
		return nil, false
	}
	rest := plist[idx+len(marker):]
	end := len(rest)
	if i := nextSiblingKeyIndex(rest); i >= 0 && i < end {
		end = i
	}
	return rest[:end], true
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
		return plistTrustsCert(adminTrustSettingsPlist(), der)
	}
	platformPresenceCheck = func(der []byte) bool {
		return bundleContainsDER(keychainCertsPEM(), der)
	}
	platformTrustReadable = func() bool {
		return trustSettingsExported(adminTrustSettingsPlist())
	}
}

// RepairSystemTrust re-asserts trust for mkcert's root CA directly, for the
// state CAPresentButUntrusted reports: the certificate sits in the keychain
// but has no trust-settings entry, most likely because a macOS update cleared
// it independent of the certificate item itself. The write restores the entry
// the trust check reads, so a repaired CA reads as trusted from the next run
// on and the prompt does not come back.
//
// This deliberately does not go through `mkcert -install`. mkcert's own
// "already installed" self-check (m.caCert.Verify against the system pool)
// is susceptible to the false positive plistTrustsCert exists to avoid: a
// self-signed root cryptographically verifies against itself regardless of
// its actual trust settings, so mkcert may silently skip re-establishing
// trust in exactly the state this function exists to fix. Running the
// add-trusted-cert half of mkcert's own install directly sidesteps that
// self-check. macOS authorizes the write itself, through the same admin
// dialog `mkcert -install` raises, so the caller is expected to have
// announced it; with no window server to draw that dialog (over ssh, say)
// the command fails and the caller reports it rather than looping.
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
