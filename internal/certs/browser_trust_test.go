package certs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserTrustAvailable(t *testing.T) {
	orig := certutilLookup
	defer func() { certutilLookup = orig }()

	certutilLookup = func() bool { return true }
	if !BrowserTrustAvailable() {
		t.Fatal("want available when certutil is present")
	}

	certutilLookup = func() bool { return false }
	if BrowserTrustAvailable() {
		t.Fatal("want unavailable when certutil is missing")
	}
}

// browserTrustFixture points the CA and store lookups at a generated CA and
// returns its directory, so each case only has to say what the stores answer.
func browserTrustFixture(t *testing.T) (caDir string, pem []byte) {
	t.Helper()
	origLookup, origRoot, origDirs, origPEM := certutilLookup, caRootFunc, nssDBDirs, nssCertPEM
	t.Cleanup(func() {
		certutilLookup, caRootFunc, nssDBDirs, nssCertPEM = origLookup, origRoot, origDirs, origPEM
	})
	certutilLookup = func() bool { return true }
	caDir = t.TempDir()
	pem = writeTestCA(t, caDir)
	caRootFunc = func() (string, error) { return caDir, nil }
	return caDir, pem
}

func TestBrowserStoresMissingCANamesTheStoreWithoutIt(t *testing.T) {
	_, certPEM := browserTrustFixture(t)
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb", "/home/u/.mozilla/firefox/p"} }
	nssCertPEM = func(dir, nickname string) []byte {
		if dir == "/home/u/.pki/nssdb" {
			return certPEM
		}
		return nil
	}

	missing := BrowserStoresMissingCA()
	if len(missing) != 1 || missing[0] != "/home/u/.mozilla/firefox/p" {
		t.Fatalf("BrowserStoresMissingCA() = %v, want only the profile that lacks the CA", missing)
	}
}

func TestBrowserStoresMissingCAEmptyWhenEveryStoreHasIt(t *testing.T) {
	_, certPEM := browserTrustFixture(t)
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb", "/home/u/.mozilla/firefox/p"} }
	nssCertPEM = func(dir, nickname string) []byte { return certPEM }

	if missing := BrowserStoresMissingCA(); len(missing) != 0 {
		t.Fatalf("BrowserStoresMissingCA() = %v, want none", missing)
	}
}

// A certificate under the nickname lerd is looking for is not automatically the
// CA lerd signs with. A CA regenerated after a store imported the old one is
// exactly this shape, and reporting it trusted is what the presence check did.
func TestBrowserStoresMissingCARejectsADifferentCertificate(t *testing.T) {
	browserTrustFixture(t)
	other := writeTestCA(t, t.TempDir())
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
	nssCertPEM = func(dir, nickname string) []byte { return other }

	if missing := BrowserStoresMissingCA(); len(missing) != 1 {
		t.Fatalf("BrowserStoresMissingCA() = %v, want the store holding a different certificate", missing)
	}
}

// Three ways the question cannot be asked at all. None of them is a finding: a
// check that could not run has not found anything wrong.
func TestBrowserStoresMissingCAReportsNothingItCouldNotAsk(t *testing.T) {
	t.Run("no certutil", func(t *testing.T) {
		browserTrustFixture(t)
		certutilLookup = func() bool { return false }
		nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
		nssCertPEM = func(dir, nickname string) []byte { return nil }
		if missing := BrowserStoresMissingCA(); len(missing) != 0 {
			t.Fatalf("BrowserStoresMissingCA() = %v, want none without certutil to ask with", missing)
		}
	})

	t.Run("no CA generated", func(t *testing.T) {
		browserTrustFixture(t)
		caRootFunc = func() (string, error) { return t.TempDir(), nil }
		nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
		nssCertPEM = func(dir, nickname string) []byte { return nil }
		if missing := BrowserStoresMissingCA(); len(missing) != 0 {
			t.Fatalf("BrowserStoresMissingCA() = %v, want none when there is no CA to look for", missing)
		}
	})

	t.Run("no browser store", func(t *testing.T) {
		browserTrustFixture(t)
		nssDBDirs = func() []string { return nil }
		if missing := BrowserStoresMissingCA(); len(missing) != 0 {
			t.Fatalf("BrowserStoresMissingCA() = %v, want none on a machine with no browser store", missing)
		}
	})
}

// mkcert stores the CA under its own label and the certificate's serial in
// decimal, and lerd has to ask for that exact nickname to find it.
func TestCANSSNicknameMatchesMkcert(t *testing.T) {
	caDir := t.TempDir()
	writeTestCA(t, caDir)
	der := firstCertDER(readFileNoErr(filepath.Join(caDir, "rootCA.pem")))

	if got, want := caNSSNickname(der), "mkcert development CA 1"; got != want {
		t.Fatalf("caNSSNickname() = %q, want %q", got, want)
	}
	if got := caNSSNickname([]byte("not a certificate")); got != "" {
		t.Fatalf("caNSSNickname() = %q for unparseable input, want empty", got)
	}
}

// Only a directory holding a database is a store. Naming one that does not
// exist would report every machine without Firefox as missing browser trust.
func TestNSSDBPresentRequiresADatabase(t *testing.T) {
	dir := t.TempDir()
	if nssDBPresent(dir) {
		t.Error("an empty directory is not an NSS store")
	}
	if err := os.WriteFile(filepath.Join(dir, "cert9.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !nssDBPresent(dir) {
		t.Error("a directory with cert9.db is an NSS store")
	}
}

// Only stores mkcert itself writes may be reported. A Flatpak Firefox profile is
// not one of them in the pinned mkcert, so naming it would leave doctor warning
// about a store `lerd dns:repair` can never fill.
func TestNSSDBDirsOnlyReportsStoresMkcertWritesTo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	native := filepath.Join(home, ".mozilla", "firefox", "abc.default")
	flatpak := filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox", "abc.default")
	for _, dir := range []string{native, flatpak} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cert9.db"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dirs := nssDBDirs()
	if len(dirs) != 1 || dirs[0] != native {
		t.Fatalf("nssDBDirs() = %v, want only %s", dirs, native)
	}
}
