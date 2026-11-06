package certs

import (
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// certutilLookup reports whether certutil is resolvable on PATH. Seam for tests.
var certutilLookup = func() bool {
	_, err := exec.LookPath("certutil")
	return err == nil
}

// BrowserTrustAvailable reports whether certutil (nss-tools) is there to put the
// root CA into the browser NSS stores. Without it only the system store trusts
// .test, so curl and PHP are fine but Firefox and Chrome warn.
func BrowserTrustAvailable() bool {
	return certutilLookup()
}

// nssDBDirs returns the existing NSS databases lerd writes the CA into: the
// Chromium store and the native and snap Firefox profiles, as mkcert did. A
// sandboxed (Flatpak) store is left out, since no repair could ever reach it.
var nssDBDirs = func() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, dir := range []string{
		filepath.Join(home, ".pki", "nssdb"),
		filepath.Join(home, "snap", "chromium", "current", ".pki", "nssdb"),
		"/etc/pki/nssdb",
	} {
		if nssDBPresent(dir) {
			dirs = append(dirs, dir)
		}
	}
	for _, pattern := range []string{
		filepath.Join(home, ".mozilla", "firefox", "*"),
		filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles", "*"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox", "*"),
	} {
		matches, _ := filepath.Glob(pattern)
		for _, dir := range matches {
			if nssDBPresent(dir) {
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}

// nssDBPresent reports whether dir holds an NSS certificate database. Only the
// sql-backed cert9.db counts: every browser that still ships has moved to it,
// and the legacy format needs a different certutil prefix to read at all.
func nssDBPresent(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "cert9.db"))
	return err == nil
}

// nssCertPEM exports the certificate stored under nickname in the NSS database
// at dir, or nothing when the store has none. Overridable in tests.
var nssCertPEM = func(dir, nickname string) []byte {
	out, err := exec.Command("certutil", "-d", "sql:"+dir, "-L", "-n", nickname, "-a").Output()
	if err != nil {
		return nil
	}
	return out
}

// nssNicknames lists every nickname in the NSS database at dir. certutil can
// only export one named certificate at a time, so this is the way to walk them.
var nssNicknames = func(dir string) []string {
	out, err := exec.Command("certutil", "-d", "sql:"+dir, "-L").Output()
	if err != nil {
		return nil
	}
	return parseNSSNicknames(string(out))
}

// nssListingEntry matches a row of certutil -L: a nickname, which may hold
// spaces, then the three comma-separated trust fields.
var nssListingEntry = regexp.MustCompile(`^(.*\S)\s+[a-zA-Z]*,[a-zA-Z]*,[a-zA-Z]*\s*$`)

func parseNSSNicknames(out string) []string {
	var nicknames []string
	for _, line := range strings.Split(out, "\n") {
		if m := nssListingEntry.FindStringSubmatch(line); m != nil {
			nicknames = append(nicknames, m[1])
		}
	}
	return nicknames
}

// nssNicknameHolding returns the nickname the store at dir keeps the root CA
// under, or "" when it has none. NSS stores a certificate once, so a CA imported
// by hand or by an older mkcert stays under that name and no second one is added.
func nssNicknameHolding(dir, expected string, der []byte) string {
	if bundleContainsDER(nssCertPEM(dir, expected), der) {
		return expected
	}
	for _, nickname := range nssNicknames(dir) {
		if bundleContainsDER(nssCertPEM(dir, nickname), der) {
			return nickname
		}
	}
	return ""
}

// caNSSNickname is the nickname mkcert stores the root CA under: its own label
// followed by the certificate's serial in decimal. Deriving it the way mkcert
// does is what lets the lookup ask for the CA lerd signs with today rather than
// for whatever certificate happens to wear the mkcert name.
func caNSSNickname(der []byte) string {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return ""
	}
	return "mkcert development CA " + cert.SerialNumber.String()
}

// BrowserStoresMissingCA returns the NSS databases lacking the root CA lerd signs
// with, searched by DER so a regenerated CA is missing and a renamed one is not.
// Empty when every store has it, or when the question could not be asked at all.
func BrowserStoresMissingCA() []string {
	if !BrowserTrustAvailable() {
		return nil
	}
	der := rootCADER()
	if der == nil {
		return nil
	}
	nickname := caNSSNickname(der)
	if nickname == "" {
		return nil
	}
	var missing []string
	for _, dir := range nssDBDirs() {
		if nssNicknameHolding(dir, nickname, der) == "" {
			missing = append(missing, dir)
		}
	}
	return missing
}

// runCertutil runs certutil against one NSS database. Overridable in tests.
var runCertutil = func(args ...string) error {
	out, err := exec.Command("certutil", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallBrowserTrust imports the root CA into every NSS database that does not
// trust it as an SSL CA, which also restores trust flags cleared on a stored
// copy. A copy already stored keeps its nickname. Returns the stores that refused.
func InstallBrowserTrust() []error {
	if !BrowserTrustAvailable() {
		return nil
	}
	root, err := CARoot()
	if err != nil {
		return []error{err}
	}
	der := rootCADER()
	expected := caNSSNickname(der)
	if expected == "" {
		return nil
	}
	var errs []error
	for _, dir := range nssDBDirs() {
		nickname := nssNicknameHolding(dir, expected, der)
		if nickname == "" {
			nickname = expected
		}
		// The check mkcert -install ran: verify the CA for SSL CA use.
		if runCertutil("-V", "-d", "sql:"+dir, "-u", "L", "-n", nickname) == nil {
			continue
		}
		if err := runCertutil("-A", "-d", "sql:"+dir, "-t", "C,,", "-n", nickname, "-i", filepath.Join(root, caCertName)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", dir, err))
			continue
		}
		// certutil -A can exit 0 without adding anything, so only -V proves it took.
		if err := runCertutil("-V", "-d", "sql:"+dir, "-u", "L", "-n", nickname); err != nil {
			errs = append(errs, fmt.Errorf("%s: the import did not take: %w", dir, err))
		}
	}
	return errs
}

// RemoveBrowserTrust deletes the root CA from every NSS database holding it.
func RemoveBrowserTrust() {
	if !BrowserTrustAvailable() {
		return
	}
	der := rootCADER()
	expected := caNSSNickname(der)
	if expected == "" {
		return
	}
	for _, dir := range nssDBDirs() {
		if nickname := nssNicknameHolding(dir, expected, der); nickname != "" {
			runCertutil("-D", "-d", "sql:"+dir, "-n", nickname) //nolint:errcheck
		}
	}
}
