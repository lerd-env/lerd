//go:build darwin

package certs

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"testing"
)

// TestCATrustedDarwinKeychain exercises CATrusted with platformTrustCheck
// wired the way it actually is in production (init() wires it to
// plistTrustsCert over adminTrustSettingsPlist), not the bare keychain
// presence check it used before trust-settings parsing replaced it.
func TestCATrustedDarwinKeychain(t *testing.T) {
	origRoot, origPaths, origPlist, origPlat := caRootFunc, caTrustPaths, adminTrustSettingsPlist, platformTrustCheck
	t.Cleanup(func() {
		caRootFunc, caTrustPaths, adminTrustSettingsPlist, platformTrustCheck = origRoot, origPaths, origPlist, origPlat
	})
	platformTrustCheck = func(der []byte) bool { return plistTrustsCert(adminTrustSettingsPlist(), der) }

	caDir := t.TempDir()
	certPEM := writeTestCA(t, caDir)
	certDER := firstCertDER(certPEM)
	caRootFunc = func() (string, error) { return caDir, nil }
	caTrustPaths = nil // no Linux bundles: the keychain is the only trust source

	t.Run("explicit trust settings, the shape mkcert writes", func(t *testing.T) {
		adminTrustSettingsPlist = func() []byte { return trustExport(trustEntry(certDER, trustRootSettings)) }
		if !CATrusted() {
			t.Fatal("expected CA reported trusted when its entry carries an explicit trustRoot result")
		}
	})

	t.Run("no trust settings array, the shape add-trusted-cert writes", func(t *testing.T) {
		adminTrustSettingsPlist = func() []byte { return trustExport(trustEntry(certDER, "")) }
		if !CATrusted() {
			t.Fatal("an entry with no trustSettings array means trust as a root, not untrusted")
		}
	})

	t.Run("explicitly denied", func(t *testing.T) {
		adminTrustSettingsPlist = func() []byte { return trustExport(trustEntry(certDER, denySettings)) }
		if CATrusted() {
			t.Fatal("expected CA reported not trusted when every trust setting denies it")
		}
	})

	t.Run("absent from the export entirely (the drifted state)", func(t *testing.T) {
		other := firstCertDER(writeTestCA(t, t.TempDir())) // a different CA
		adminTrustSettingsPlist = func() []byte { return trustExport(trustEntry(other, trustRootSettings)) }
		if CATrusted() {
			t.Fatal("expected CA reported not trusted when absent from the trust-settings export")
		}
	})

	// An export that did not answer has said nothing about trust either way, and
	// calling that untrusted announces a sudo prompt and a full interactive
	// mkcert -install on a host where the export keeps failing.
	t.Run("export unreadable", func(t *testing.T) {
		adminTrustSettingsPlist = func() []byte { return nil }
		if !CATrusted() {
			t.Fatal("an export that could not be read must not be reported as untrusted")
		}
	})
}

const (
	// The array mkcert writes for its own root through trust-settings-import.
	trustRootSettings = `<key>trustSettings</key><array><dict>
		<key>kSecTrustSettingsPolicyName</key><string>sslServer</string>
		<key>kSecTrustSettingsResult</key><integer>1</integer>
	</dict></array>`
	// kSecTrustSettingsResultDeny: the one shape that really is untrusted.
	denySettings = `<key>trustSettings</key><array><dict>
		<key>kSecTrustSettingsPolicyName</key><string>sslServer</string>
		<key>kSecTrustSettingsResult</key><integer>3</integer>
	</dict></array>`
)

// trustEntry renders one trustList entry keyed by der's SHA-1 fingerprint the
// way `security trust-settings-export -d` keys it, carrying settings verbatim.
// An entry with no trustSettings array is the ordinary shape of a root added
// by `security add-trusted-cert`: on a real machine here, a trusted Laravel
// Valet CA and a trusted Blizzard local cert both export that way, and only
// the mkcert CA carries an explicit array, because mkcert writes one itself
// through trust-settings-import.
func trustEntry(der []byte, settings string) string {
	sum := sha1.Sum(der)
	key := strings.ToUpper(hex.EncodeToString(sum[:]))
	return "<key>" + key + "</key><dict>" +
		"<key>issuerName</key><data>AAAA</data>" +
		"<key>modDate</key><date>2026-07-30T09:00:00Z</date>" +
		"<key>serialNumber</key><data>BBBB</data>" +
		settings + "</dict>"
}

// trustExport wraps entries in the plist `security trust-settings-export -d`
// produces.
func trustExport(entries ...string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>trustList</key>
	<dict>` + strings.Join(entries, "") + `</dict>
	<key>trustVersion</key>
	<integer>1</integer>
</dict>
</plist>`)
}

// plistTrustsCert's byte-search logic is the part carrying real risk (a wrong
// boundary could read a neighbouring entry's settings as this one's, or miss
// the entry entirely), so it gets direct coverage on top of
// TestCATrustedDarwinKeychain's higher-level exercise.
func TestPlistTrustsCert(t *testing.T) {
	der := firstCertDER(writeTestCA(t, t.TempDir()))
	otherDER := firstCertDER(writeTestCA(t, t.TempDir()))

	if !plistTrustsCert(trustExport(trustEntry(der, trustRootSettings)), der) {
		t.Error("an explicit trustRoot result must be reported as trusted")
	}
	if !plistTrustsCert(trustExport(trustEntry(der, "")), der) {
		t.Error("an entry with no trustSettings array must be reported as trusted")
	}
	if plistTrustsCert(trustExport(trustEntry(der, denySettings)), der) {
		t.Error("an explicitly denied entry must not be reported as trusted")
	}
	if plistTrustsCert(trustExport(trustEntry(otherDER, trustRootSettings)), der) {
		t.Error("a different certificate's entry must not count for der")
	}
	if plistTrustsCert(nil, der) {
		t.Error("a nil export (security failed, or nothing trusted yet) must report not trusted")
	}

	// A denied neighbour immediately before the target is the case a wrong
	// block boundary gets wrong in the direction that matters: reading the
	// neighbour's deny as the target's would prompt a repair on every run.
	t.Run("denied neighbour before the target", func(t *testing.T) {
		denied := firstCertDER(writeTestCA(t, t.TempDir()))
		plist := trustExport(trustEntry(denied, denySettings), trustEntry(der, ""))
		if plistTrustsCert(plist, denied) {
			t.Error("the denied neighbour must not be reported as trusted")
		}
		if !plistTrustsCert(plist, der) {
			t.Error("the target must stay trusted despite the denied neighbour before it")
		}
	})
}

// A trust decision may only be drawn from an export that actually parsed.
// Reporting an unreadable export as "not trusted" would make every install
// announce and re-run the repair, the failure mode issue #1101 is about.
func TestTrustSettingsExported(t *testing.T) {
	der := firstCertDER(writeTestCA(t, t.TempDir()))
	if !trustSettingsExported(trustExport(trustEntry(der, ""))) {
		t.Error("a real export must read as exported")
	}
	if trustSettingsExported(nil) {
		t.Error("a failed export must not read as exported")
	}
	if trustSettingsExported([]byte("bplist00\x00\x01")) {
		t.Error("output that is not the XML export must not read as exported")
	}
}

func TestAddTrustedCertArgs(t *testing.T) {
	got := addTrustedCertArgs("/Library/Keychains/System.keychain", "/ca/rootCA.pem")
	want := []string{"add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", "/ca/rootCA.pem"}
	if len(got) != len(want) {
		t.Fatalf("addTrustedCertArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("addTrustedCertArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
