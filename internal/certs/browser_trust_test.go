package certs

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
	origLookup, origRoot, origDirs, origPEM, origNicks := certutilLookup, caRootFunc, nssDBDirs, nssCertPEM, nssNicknames
	t.Cleanup(func() {
		certutilLookup, caRootFunc, nssDBDirs, nssCertPEM, nssNicknames = origLookup, origRoot, origDirs, origPEM, origNicks
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

// stubCertutil records every certutil call; verify answers true only for the
// stores in trusted, the way certutil -V does for a CA trusted as an SSL CA, and
// an import makes its store trusted.
func stubCertutil(t *testing.T, trusted ...string) *[][]string {
	t.Helper()
	orig := runCertutil
	t.Cleanup(func() { runCertutil = orig })
	var calls [][]string
	runCertutil = func(args ...string) error {
		calls = append(calls, args)
		store := strings.TrimPrefix(args[2], "sql:")
		if args[0] == "-A" {
			trusted = append(trusted, store)
		}
		if args[0] == "-V" && !slices.Contains(trusted, store) {
			return errors.New("certificate not trusted")
		}
		return nil
	}
	return &calls
}

// stubNSSStore serves the given nickname to certificate entries as every store's
// contents, through both the listing and the per-nickname export.
func stubNSSStore(t *testing.T, entries map[string][]byte) {
	t.Helper()
	nssNicknames = func(string) []string { return slices.Collect(maps.Keys(entries)) }
	nssCertPEM = func(_, nickname string) []byte { return entries[nickname] }
}

func imports(calls [][]string) [][]string {
	var out [][]string
	for _, c := range calls {
		if c[0] == "-A" {
			out = append(out, c)
		}
	}
	return out
}

// A store already trusting the CA is left alone; the other gets the import,
// under the nickname BrowserStoresMissingCA reads back.
func TestInstallBrowserTrustImportsWhereTheCAIsNotTrusted(t *testing.T) {
	caDir, certPEM := browserTrustFixture(t)
	calls := stubCertutil(t, "/home/u/.pki/nssdb")
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb", "/home/u/.mozilla/firefox/p"} }

	if errs := InstallBrowserTrust(); len(errs) != 0 {
		t.Fatalf("InstallBrowserTrust() = %v", errs)
	}
	nickname := caNSSNickname(firstCertDER(certPEM))
	want := []string{"-A", "-d", "sql:/home/u/.mozilla/firefox/p", "-t", "C,,", "-n", nickname, "-i", filepath.Join(caDir, "rootCA.pem")}
	if got := imports(*calls); len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("imports = %v, want one %v", got, want)
	}
}

// A store that still holds the CA but whose trust flags were cleared is what
// comparing certificate bytes cannot see, so it must be imported again.
func TestInstallBrowserTrustRestoresClearedTrust(t *testing.T) {
	_, certPEM := browserTrustFixture(t)
	calls := stubCertutil(t)
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
	nssCertPEM = func(dir, nickname string) []byte { return certPEM }

	InstallBrowserTrust()
	if got := imports(*calls); len(got) != 1 {
		t.Fatalf("imports = %v, want the store with cleared trust re-imported", got)
	}
}

func TestRemoveBrowserTrustDeletesOnlyWhereItIs(t *testing.T) {
	_, certPEM := browserTrustFixture(t)
	calls := stubCertutil(t)
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb", "/home/u/.mozilla/firefox/p"} }
	nssCertPEM = func(dir, nickname string) []byte {
		if dir == "/home/u/.pki/nssdb" {
			return certPEM
		}
		return nil
	}

	RemoveBrowserTrust()
	nickname := caNSSNickname(firstCertDER(certPEM))
	want := []string{"-D", "-d", "sql:/home/u/.pki/nssdb", "-n", nickname}
	if len(*calls) != 1 || !slices.Equal((*calls)[0], want) {
		t.Fatalf("certutil calls = %v, want one %v", *calls, want)
	}
}

// A store holding the CA under another nickname, the state a hand-run certutil
// or an older mkcert leaves, has it all the same: browsers match certificates,
// not nicknames.
func TestBrowserStoresMissingCAFindsTheCAUnderAnotherNickname(t *testing.T) {
	_, certPEM := browserTrustFixture(t)
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
	stubNSSStore(t, map[string][]byte{"My own name": certPEM})

	if missing := BrowserStoresMissingCA(); len(missing) != 0 {
		t.Fatalf("BrowserStoresMissingCA() = %v, want none", missing)
	}
}

// certutil -A exits 0 without adding anything when the certificate is already
// stored under another nickname, so success has to be read back from the store.
func TestInstallBrowserTrustReportsAnImportThatDidNotTake(t *testing.T) {
	browserTrustFixture(t)
	stubCertutil(t)
	stub := runCertutil
	runCertutil = func(args ...string) error {
		if args[0] == "-A" {
			return nil // exits 0, and -V keeps failing because nothing was added
		}
		return stub(args...)
	}
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
	stubNSSStore(t, nil)

	errs := InstallBrowserTrust()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "/home/u/.pki/nssdb") {
		t.Fatalf("InstallBrowserTrust() = %v, want the store named", errs)
	}
}

func TestRemoveBrowserTrustDeletesTheCAUnderAnotherNickname(t *testing.T) {
	_, certPEM := browserTrustFixture(t)
	calls := stubCertutil(t)
	nssDBDirs = func() []string { return []string{"/home/u/.pki/nssdb"} }
	stubNSSStore(t, map[string][]byte{"My own name": certPEM})

	RemoveBrowserTrust()
	want := []string{"-D", "-d", "sql:/home/u/.pki/nssdb", "-n", "My own name"}
	if len(*calls) != 1 || !slices.Equal((*calls)[0], want) {
		t.Fatalf("certutil calls = %v, want one %v", *calls, want)
	}
}

func TestParseNSSNicknames(t *testing.T) {
	out := "\nCertificate Nickname                                         Trust Attributes\n" +
		"                                                             SSL,S/MIME,JAR/XPI\n\n" +
		"Lerd mkcert (someone)                                        C,,  \n" +
		"cleared                                                      ,,   \n" +
		"signer                                                       CTu,u,u\n"
	got := parseNSSNicknames(out)
	want := []string{"Lerd mkcert (someone)", "cleared", "signer"}
	if !slices.Equal(got, want) {
		t.Fatalf("parseNSSNicknames() = %q, want %q", got, want)
	}
	if got := parseNSSNicknames(""); len(got) != 0 {
		t.Fatalf("parseNSSNicknames(empty) = %q, want none", got)
	}
}

// The behaviour the fix rests on is certutil's, so this runs the real one
// against a throwaway database instead of a stub of what it was seen doing.
func TestBrowserTrustWithRealCertutilAndAForeignNickname(t *testing.T) {
	if _, err := exec.LookPath("certutil"); err != nil {
		t.Skip("certutil not installed")
	}
	caDir, _ := browserTrustFixture(t)
	db := t.TempDir()
	nssDBDirs = func() []string { return []string{db} }
	ca := filepath.Join(caDir, "rootCA.pem")
	if err := runCertutil("-N", "-d", "sql:"+db, "--empty-password"); err != nil {
		t.Fatal(err)
	}
	if err := runCertutil("-A", "-d", "sql:"+db, "-t", ",,", "-n", "My own name", "-i", ca); err != nil {
		t.Fatal(err)
	}

	if missing := BrowserStoresMissingCA(); len(missing) != 0 {
		t.Fatalf("BrowserStoresMissingCA() = %v, want none", missing)
	}
	if errs := InstallBrowserTrust(); len(errs) != 0 {
		t.Fatalf("InstallBrowserTrust() = %v", errs)
	}
	if err := runCertutil("-V", "-d", "sql:"+db, "-u", "L", "-n", "My own name"); err != nil {
		t.Fatalf("trust not restored on the stored entry: %v", err)
	}
	RemoveBrowserTrust()
	if nicks := nssNicknames(db); len(nicks) != 0 {
		t.Fatalf("store still holds %q after removal", nicks)
	}
}
