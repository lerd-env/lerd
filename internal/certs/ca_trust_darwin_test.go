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
// plistHasTrustEntry over adminTrustSettingsPlist), not the bare keychain
// presence check it used before trust-settings parsing replaced it.
func TestCATrustedDarwinKeychain(t *testing.T) {
	origRoot, origPaths, origPlist, origPlat := caRootFunc, caTrustPaths, adminTrustSettingsPlist, platformTrustCheck
	t.Cleanup(func() {
		caRootFunc, caTrustPaths, adminTrustSettingsPlist, platformTrustCheck = origRoot, origPaths, origPlist, origPlat
	})
	platformTrustCheck = func(der []byte) bool { return plistHasTrustEntry(adminTrustSettingsPlist(), der) }

	caDir := t.TempDir()
	certPEM := writeTestCA(t, caDir)
	certDER := firstCertDER(certPEM)
	caRootFunc = func() (string, error) { return caDir, nil }
	caTrustPaths = nil // no Linux bundles: the keychain is the only trust source

	t.Run("trusted in keychain", func(t *testing.T) {
		adminTrustSettingsPlist = func() []byte { return trustSettingsFixture(certDER, true) }
		if !CATrusted() {
			t.Fatal("expected CA reported trusted when its trust-settings entry carries trustSettings")
		}
	})

	t.Run("present but not trusted (the drifted state)", func(t *testing.T) {
		adminTrustSettingsPlist = func() []byte { return trustSettingsFixture(certDER, false) }
		if CATrusted() {
			t.Fatal("expected CA reported not trusted when its entry carries no trustSettings")
		}
	})

	t.Run("absent from the export entirely", func(t *testing.T) {
		other := firstCertDER(writeTestCA(t, t.TempDir())) // a different CA
		adminTrustSettingsPlist = func() []byte { return trustSettingsFixture(other, true) }
		if CATrusted() {
			t.Fatal("expected CA reported not trusted when absent from the trust-settings export")
		}
	})
}

// trustSettingsFixture renders a minimal but structurally real
// `security trust-settings-export -d` plist with one trustList entry for
// der, keyed by its SHA-1 fingerprint the way security itself keys it,
// carrying a trustSettings array iff trusted. Modelled on real output
// captured from `security trust-settings-export -d` on a machine with both a
// trusted mkcert CA and a present-but-untrusted leftover CA side by side.
func trustSettingsFixture(der []byte, trusted bool) []byte {
	sum := sha1.Sum(der)
	key := strings.ToUpper(hex.EncodeToString(sum[:]))
	entry := "<key>" + key + "</key><dict><key>issuerName</key><data>AAAA</data>"
	if trusted {
		entry += `<key>trustSettings</key><array><dict>
			<key>kSecTrustSettingsPolicyName</key><string>sslServer</string>
			<key>kSecTrustSettingsResult</key><integer>1</integer>
		</dict></array>`
	}
	entry += "</dict>"
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>trustList</key>
	<dict>` + entry + `</dict>
	<key>trustVersion</key>
	<integer>1</integer>
</dict>
</plist>`)
}

// plistHasTrustEntry's byte-search logic is the part carrying real risk (a
// wrong boundary could leak a neighbouring entry's trustSettings onto this
// one, or miss a real one), so it gets direct table-driven coverage on top
// of TestCATrustedDarwinKeychain's higher-level exercise.
func TestPlistHasTrustEntry(t *testing.T) {
	caDir := t.TempDir()
	certPEM := writeTestCA(t, caDir)
	der := firstCertDER(certPEM)
	otherDER := firstCertDER(writeTestCA(t, t.TempDir()))

	if !plistHasTrustEntry(trustSettingsFixture(der, true), der) {
		t.Error("a trusted entry must be reported as trusted")
	}
	if plistHasTrustEntry(trustSettingsFixture(der, false), der) {
		t.Error("a present entry with no trustSettings array must not be reported as trusted")
	}
	if plistHasTrustEntry(trustSettingsFixture(otherDER, true), der) {
		t.Error("a different certificate's trusted entry must not count for der")
	}
	if plistHasTrustEntry(nil, der) {
		t.Error("a nil/empty export (security failed, or nothing trusted yet) must report not trusted")
	}

	// Two real, mixed entries in one export — an untrusted one before the
	// target and a trusted one after it — is exactly the shape the bug fix is
	// for: two certificates present side by side, one drifted, one not.
	t.Run("real-shaped export with an untrusted neighbour before the trusted target", func(t *testing.T) {
		untrusted := firstCertDER(writeTestCA(t, t.TempDir()))
		sumU, sumT := sha1.Sum(untrusted), sha1.Sum(der)
		keyU := strings.ToUpper(hex.EncodeToString(sumU[:]))
		keyT := strings.ToUpper(hex.EncodeToString(sumT[:]))
		plist := []byte(`<dict><key>trustList</key><dict>` +
			"<key>" + keyU + "</key><dict><key>issuerName</key><data>AAAA</data></dict>" +
			"<key>" + keyT + "</key><dict><key>issuerName</key><data>BBBB</data>" +
			`<key>trustSettings</key><array><dict><key>kSecTrustSettingsResult</key><integer>1</integer></dict></array></dict>` +
			`</dict><key>trustVersion</key><integer>1</integer></dict>`)
		if plistHasTrustEntry(plist, untrusted) {
			t.Error("the untrusted neighbour must not be reported as trusted")
		}
		if !plistHasTrustEntry(plist, der) {
			t.Error("the trusted target must be reported as trusted despite the untrusted neighbour before it")
		}
	})
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
