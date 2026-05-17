package certs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ── MkcertPath ────────────────────────────────────────────────────────────────

func TestMkcertPath_usesDataDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	got := MkcertPath()
	want := filepath.Join(tmp, "lerd", "bin", "mkcert")
	if got != want {
		t.Errorf("MkcertPath() = %q, want %q", got, want)
	}
}

// ── CertExists ────────────────────────────────────────────────────────────────

func TestCertExists_returnsFalseWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	if CertExists("myapp.test") {
		t.Error("expected false for non-existent cert")
	}
}

func TestCertExists_returnsTrueWhenPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Create the expected cert file path
	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	os.MkdirAll(certsDir, 0755)
	os.WriteFile(filepath.Join(certsDir, "myapp.test.crt"), []byte("fake cert"), 0644)

	if !CertExists("myapp.test") {
		t.Error("expected true when cert file exists")
	}
}

func TestCertExists_onlyCrtRequired(t *testing.T) {
	// CertExists checks for .crt only, not .key
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	os.MkdirAll(certsDir, 0755)
	// .crt exists, no .key
	os.WriteFile(filepath.Join(certsDir, "site.test.crt"), []byte("fake cert"), 0644)

	if !CertExists("site.test") {
		t.Error("expected true when only .crt file exists")
	}
}

// ── IssueCert vs IssueCertForce semantics ────────────────────────────────────

// IssueCert is documented as a no-op when the cert and key already exist on
// disk. Pins the contract so callers that mutate the SAN list (domain add,
// edit, remove) keep using IssueCertForce; using IssueCert would silently
// preserve a stale cert and the browser would reject the new hostname with
// ERR_CERT_AUTHORITY_INVALID. Regression test for the bug where adding a
// secondary domain to a secured site never widened the cert's SAN list.
func TestIssueCert_skipsWhenCertExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Fake mkcert that records the SAN list it was asked for so we can
	// detect whether it was invoked at all.
	fakeMkcert := `#!/bin/sh
CRT=""
KEY=""
SANS=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -cert-file) shift; CRT="$1" ;;
    -key-file)  shift; KEY="$1" ;;
    *) SANS="$SANS $1" ;;
  esac
  shift
done
printf '%s' "$SANS" > "$CRT"
printf 'KEY' > "$KEY"
`
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(fakeMkcert), 0755); err != nil {
		t.Fatal(err)
	}

	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(certsDir, "site.test.crt")
	keyPath := filepath.Join(certsDir, "site.test.key")
	stale := []byte("STALE-CERT-ONE-DOMAIN")
	if err := os.WriteFile(certPath, stale, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("STALE-KEY"), 0644); err != nil {
		t.Fatal(err)
	}

	// IssueCert should leave the stale cert untouched, even though the SAN
	// list it was asked for includes a brand-new domain.
	if err := IssueCert("site.test", []string{"site.test", "extra.test"}, certsDir); err != nil {
		t.Fatalf("IssueCert returned %v", err)
	}
	got, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(stale) {
		t.Errorf("IssueCert overwrote existing cert; got %q want %q", got, stale)
	}

	// IssueCertForce must overwrite the file with a cert whose SAN list
	// includes the new domain. The fake mkcert echoes the SAN args into
	// the cert body so we can grep for them.
	if err := IssueCertForce("site.test", []string{"site.test", "extra.test"}, certsDir); err != nil {
		t.Fatalf("IssueCertForce returned %v", err)
	}
	got, err = os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "extra.test") {
		t.Errorf("IssueCertForce did not include new domain in SAN list; cert body %q", got)
	}
	if !strings.Contains(string(got), "*.extra.test") {
		t.Errorf("IssueCertForce did not include wildcard SAN for new domain; cert body %q", got)
	}
}

// ── IssueCertForce concurrency ───────────────────────────────────────────────

// TestIssueCertForce_concurrentCallsDontCollide pins the fix for the shared
// .new tempfile race: two parallel IssueCertForce calls for the same domain
// (e.g. boot scanWorktrees + a watcher syncWorktree event firing on the same
// site) must not interleave their renames. Pre-fix both writers used a
// fixed "<primary>.crt.new" path; one would clobber the other's tempfile
// mid-write or rename a partially-flushed file. The fix uses a unique
// tempfile per goroutine.
func TestIssueCertForce_concurrentCallsDontCollide(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Fake mkcert that writes its SAN list into the cert/key tempfiles
	// so we can detect a half-written / clobbered cert. A 50ms sleep
	// widens the race window beyond filesystem-rename atomicity.
	fakeMkcert := `#!/bin/sh
CRT=""
KEY=""
SANS=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -cert-file) shift; CRT="$1" ;;
    -key-file)  shift; KEY="$1" ;;
    *) SANS="$SANS $1" ;;
  esac
  shift
done
sleep 0.05
echo "$SANS" > "$CRT"
echo "FAKE-KEY" > "$KEY"
`
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(fakeMkcert), 0755); err != nil {
		t.Fatal(err)
	}
	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatal(err)
	}

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := IssueCertForce("alpha.test", []string{"alpha.test"}, certsDir)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Errorf("concurrent IssueCertForce returned error: %v", e)
		}
	}

	// Cert must exist and contain a complete SAN write (the fake echoes
	// SANs verbatim; truncation or interleaving would break the prefix).
	body, err := os.ReadFile(filepath.Join(certsDir, "alpha.test.crt"))
	if err != nil {
		t.Fatalf("cert missing after concurrent issue: %v", err)
	}
	if !strings.Contains(string(body), "alpha.test") {
		t.Errorf("cert content corrupted by concurrent rename; got %q", body)
	}

	// No leftover temp files: each goroutine should have cleaned up its
	// own .new.* paths even on the rename-loser side.
	entries, _ := os.ReadDir(certsDir)
	for _, e := range entries {
		name := e.Name()
		if name == "alpha.test.crt" || name == "alpha.test.key" {
			continue
		}
		if strings.Contains(name, ".new") {
			t.Errorf("leftover temp file %q after concurrent issue", name)
		}
	}
}

// ── IssueCertForce atomicity ─────────────────────────────────────────────────

// TestIssueCertForce_keyRenameFailureRollsBackCert pins the cert/key
// pair atomicity guarantee: when the key rename fails after the cert
// rename succeeded, the previous cert is restored so we don't leave a
// new-cert + old-key mismatch that nginx refuses to load. Failure is
// triggered by making the fake mkcert emit a directory at the key
// tempfile path — POSIX rename(directory, regular file) → ENOTDIR.
func TestIssueCertForce_keyRenameFailureRollsBackCert(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeMkcert := `#!/bin/sh
CRT=""
KEY=""
while [ $# -gt 0 ]; do
  case "$1" in
    -cert-file) shift; CRT="$1" ;;
    -key-file)  shift; KEY="$1" ;;
  esac
  shift
done
printf 'NEW-CERT' > "$CRT"
mkdir -p "$KEY"
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(fakeMkcert), 0755); err != nil {
		t.Fatal(err)
	}

	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(certsDir, "myapp.test.crt")
	keyPath := filepath.Join(certsDir, "myapp.test.key")
	if err := os.WriteFile(certPath, []byte("OLD-CERT"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("OLD-KEY"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := IssueCertForce("myapp.test", []string{"myapp.test"}, certsDir); err == nil {
		t.Fatal("expected error when key rename fails, got nil")
	}

	gotCert, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("cert missing after rollback: %v", err)
	}
	if string(gotCert) != "OLD-CERT" {
		t.Errorf("cert not rolled back; got %q, want OLD-CERT", gotCert)
	}
	gotKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("key missing after rollback: %v", err)
	}
	if string(gotKey) != "OLD-KEY" {
		t.Errorf("key shouldn't have changed; got %q, want OLD-KEY", gotKey)
	}
}

// IssueCertForce must leave the existing cert/key intact when mkcert fails,
// otherwise a transient error would trip RepairVhosts into flipping the site
// to plain HTTP on the next start.
func TestIssueCertForce_failureLeavesExistingCertIntact(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	binDir := filepath.Join(tmp, "lerd", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Fake mkcert that exits non-zero.
	mkcertScript := "#!/bin/sh\necho 'simulated mkcert failure' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(mkcertScript), 0755); err != nil {
		t.Fatal(err)
	}

	certsDir := filepath.Join(tmp, "lerd", "certs", "sites")
	if err := os.MkdirAll(certsDir, 0755); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(certsDir, "myapp.test.crt")
	keyPath := filepath.Join(certsDir, "myapp.test.key")
	originalCert := []byte("EXISTING-CERT-PEM")
	originalKey := []byte("EXISTING-KEY-PEM")
	if err := os.WriteFile(certPath, originalCert, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, originalKey, 0644); err != nil {
		t.Fatal(err)
	}

	err := IssueCertForce("myapp.test", []string{"myapp.test", "feat-x.myapp.test"}, certsDir)
	if err == nil {
		t.Fatal("expected error when mkcert fails, got nil")
	}

	gotCert, readErr := os.ReadFile(certPath)
	if readErr != nil {
		t.Fatalf("existing cert was deleted after failure: %v", readErr)
	}
	if string(gotCert) != string(originalCert) {
		t.Errorf("cert overwritten on failure path; got %q want %q", gotCert, originalCert)
	}
	gotKey, readErr := os.ReadFile(keyPath)
	if readErr != nil {
		t.Fatalf("existing key was deleted after failure: %v", readErr)
	}
	if string(gotKey) != string(originalKey) {
		t.Errorf("key overwritten on failure path")
	}

	// Temp paths must be cleaned up too — leftover .new files would
	// confuse a subsequent successful reissue.
	if _, err := os.Stat(certPath + ".new"); !os.IsNotExist(err) {
		t.Error("temp .crt.new not cleaned up after failure")
	}
	if _, err := os.Stat(keyPath + ".new"); !os.IsNotExist(err) {
		t.Error("temp .key.new not cleaned up after failure")
	}
}
