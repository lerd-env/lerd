package cli

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/platform"
	lerdUpdate "github.com/geodro/lerd/internal/update"
)

// ── package-managed detection ─────────────────────────────────────────────────

func TestIsSystemPackageManaged(t *testing.T) {
	// The /usr prefixes only mean "a package manager put it there" on Linux;
	// on macOS /usr/local is an ordinary prefix and Intel Homebrew's own.
	linuxOnly := runtime.GOOS == "linux"
	cases := []struct {
		path string
		want bool
	}{
		{"/usr/bin/lerd", linuxOnly},
		{"/usr/local/bin/lerd", linuxOnly},
		{"/var/usrlocal/bin/lerd", linuxOnly},
		{"/nix/store/abc123-lerd-1.30.0/bin/lerd", true},
		{"/home/george/.local/bin/lerd", false},
		{"/opt/lerd/lerd", false},
		{"/tmp/lerd", false},
	}
	for _, c := range cases {
		if got := isSystemPackageManaged(c.path); got != c.want {
			t.Errorf("isSystemPackageManaged(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestPackageManagerHints(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	stub := func(present ...string) {
		lookPath = func(bin string) (string, error) {
			for _, p := range present {
				if bin == p {
					return "/usr/bin/" + bin, nil
				}
			}
			return "", os.ErrNotExist
		}
	}

	stub("dnf")
	if got := packageManagerUpdateHint("/usr/bin/lerd"); got != "sudo dnf upgrade lerd" {
		t.Errorf("dnf update hint = %q", got)
	}
	if got := packageManagerRemoveHint("/usr/bin/lerd"); got != "sudo dnf remove lerd" {
		t.Errorf("dnf remove hint = %q", got)
	}

	stub("pacman")
	if got := packageManagerUpdateHint("/usr/bin/lerd"); got != "sudo pacman -Syu lerd" {
		t.Errorf("pacman update hint = %q", got)
	}

	// Atomic Fedora has both; rpm-ostree owns layered packages, so it wins.
	stub("rpm-ostree", "dnf")
	if got := packageManagerUpdateHint("/usr/bin/lerd"); got != "rpm-ostree upgrade" {
		t.Errorf("rpm-ostree update hint = %q", got)
	}

	// A Nix store path decides by itself, whatever is on PATH.
	stub("apt")
	if got := packageManagerUpdateHint("/nix/store/abc-lerd/bin/lerd"); !strings.Contains(got, "nix profile upgrade") {
		t.Errorf("nix update hint = %q", got)
	}

	// No known package manager present falls back to a generic sentence.
	stub("")
	if got := packageManagerUpdateHint("/usr/bin/lerd"); !strings.Contains(got, "package manager") {
		t.Errorf("fallback update hint = %q", got)
	}
}

// ── stripV ───────────────────────────────────────────────────────────────────

func TestStripV(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.1.0", "0.1.0"},
		{"", ""},
		{"v", ""},
	}
	for _, c := range cases {
		got := stripV(c.in)
		if got != c.want {
			t.Errorf("stripV(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── isHomebrewManaged ──────────────────────────────────────────────────────────

func TestIsHomebrewManaged(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/opt/homebrew/Cellar/lerd/1.25.0/bin/lerd", true},
		{"/usr/local/Cellar/lerd/1.25.0/bin/lerd", true},
		// The formula ships Linux bottles too, so Linuxbrew must be recognised
		// or `lerd update` would rename a new binary into brew's Cellar.
		{"/home/linuxbrew/.linuxbrew/Cellar/lerd/1.31.0/bin/lerd", true},
		{"/Users/me/.local/bin/lerd", false},
		{"/home/me/.local/bin/lerd", false},
		{"/usr/local/bin/lerd", false},
	}
	for _, c := range cases {
		if got := isHomebrewManaged(c.path); got != c.want {
			t.Errorf("isHomebrewManaged(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// ── fetchLatestVersion ───────────────────────────────────────────────────────

func TestFetchLatestVersion_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String()+"/tag/v1.2.3", http.StatusFound)
	}))
	defer srv.Close()

	orig := lerdUpdate.ReleaseBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.ReleaseBaseURLs = orig }()

	got, err := lerdUpdate.FetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.2.3" {
		t.Errorf("got %q, want v1.2.3", got)
	}
}

func TestFetchLatestVersion_withoutVPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String()+"/tag/0.9.0", http.StatusFound)
	}))
	defer srv.Close()

	orig := lerdUpdate.ReleaseBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.ReleaseBaseURLs = orig }()

	got, err := lerdUpdate.FetchLatestVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "0.9.0" {
		t.Errorf("got %q, want 0.9.0", got)
	}
}

func TestFetchLatestVersion_notFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := lerdUpdate.ReleaseBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.ReleaseBaseURLs = orig }()

	_, err := lerdUpdate.FetchLatestVersion()
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestFetchLatestVersion_emptyTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String()+"/tag/", http.StatusFound)
	}))
	defer srv.Close()

	orig := lerdUpdate.ReleaseBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.ReleaseBaseURLs = orig }()

	_, err := lerdUpdate.FetchLatestVersion()
	if err == nil {
		t.Fatal("expected error for empty tag, got nil")
	}
}

func TestFetchLatestVersion_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := lerdUpdate.ReleaseBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.ReleaseBaseURLs = orig }()

	_, err := lerdUpdate.FetchLatestVersion()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// ── downloadReleaseBinary ────────────────────────────────────────────────────

// makeFakeTarGz creates a .tar.gz archive in dir containing a file named "lerd"
// with the given content.
func makeFakeTarGz(t *testing.T, dir, content string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "lerd_0.1.0_linux_amd64.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	data := []byte(content)
	tw.WriteHeader(&tar.Header{Name: "lerd", Mode: 0755, Size: int64(len(data))})
	tw.Write(data)
	return archivePath
}

func TestDownloadReleaseBinary_success(t *testing.T) {
	// Build a fake tar.gz to serve
	tmp := t.TempDir()
	makeFakeTarGz(t, tmp, "#!/bin/sh\necho lerd")

	archiveBytes, err := os.ReadFile(filepath.Join(tmp, "lerd_0.1.0_linux_amd64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write(archiveBytes)
	}))
	defer srv.Close()

	orig := githubDownloadBases
	githubDownloadBases = func() []string { return []string{srv.URL} }
	defer func() { githubDownloadBases = orig }()

	binary, cleanup, err := downloadReleaseBinary("v0.1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(binary); err != nil {
		t.Errorf("binary not found at %s: %v", binary, err)
	}
}

func TestDownloadReleaseBinary_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := githubDownloadBases
	githubDownloadBases = func() []string { return []string{srv.URL} }
	defer func() { githubDownloadBases = orig }()

	_, cleanup, err := downloadReleaseBinary("v0.1.0")
	cleanup()
	if err == nil {
		t.Fatal("expected error for 404 download, got nil")
	}
}

// The version reaches here from a release redirect, so the download must not
// trust it: it is joined onto a temp dir as a filename and sent as a URL.
func TestDownloadReleaseBinary_rejectsUnsafeVersion(t *testing.T) {
	var requested int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requested++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := githubDownloadBases
	githubDownloadBases = func() []string { return []string{srv.URL} }
	defer func() { githubDownloadBases = orig }()

	for _, ver := range []string{"v../../etc/passwd", "v1.0.0/../..", "v1.0.0 rm -rf /"} {
		_, cleanup, err := downloadReleaseBinary(ver)
		cleanup()
		if err == nil {
			t.Errorf("version %q was accepted, want a refusal", ver)
		}
	}
	if requested != 0 {
		t.Errorf("an unsafe version reached the network %d times", requested)
	}
}

// ── copyFile ─────────────────────────────────────────────────────────────────

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	if err := os.WriteFile(src, []byte("hello lerd"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst, 0755); err != nil {
		t.Fatalf("copyFile error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello lerd" {
		t.Errorf("got %q, want %q", got, "hello lerd")
	}

	info, _ := os.Stat(dst)
	if info.Mode() != 0755 {
		t.Errorf("mode = %v, want 0755", info.Mode())
	}
}

func TestCopyFile_missingSource(t *testing.T) {
	tmp := t.TempDir()
	err := copyFile(filepath.Join(tmp, "nope"), filepath.Join(tmp, "dst"), 0644)
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

// ── runUpdate (integration-style) ────────────────────────────────────────────

// stubLatestRelease makes v1.0.0 the newest release, off the network.
func stubLatestRelease(t *testing.T) {
	t.Helper()
	// A real config with update.beta on sends the check to GitHub's prerelease
	// list, which this stub does not cover, and the test then installs the
	// published beta over the real binary.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	origBeta := lerdUpdate.BetaChannel
	lerdUpdate.BetaChannel = func() bool { return false }
	t.Cleanup(func() { lerdUpdate.BetaChannel = origBeta })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String()+"/tag/v1.0.0", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	orig := lerdUpdate.ReleaseBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{srv.URL} }
	t.Cleanup(func() { lerdUpdate.ReleaseBaseURLs = orig })
}

// stubFetchRelease counts release downloads, which serve an empty directory so
// nothing lands beside the test binary.
func stubFetchRelease(t *testing.T) *int {
	t.Helper()
	orig := fetchRelease
	t.Cleanup(func() { fetchRelease = orig })
	fetched := 0
	extracted := t.TempDir()
	fetchRelease = func(string) (string, func(), error) {
		fetched++
		return extracted, func() {}, nil
	}
	return &fetched
}

func TestRunUpdate_alreadyLatest(t *testing.T) {
	stubLatestRelease(t)
	fetched := stubFetchRelease(t)
	prev := platform.Current.WindowsSide
	t.Cleanup(func() { platform.Current.WindowsSide = prev })
	platform.Current.WindowsSide = false

	if err := runUpdate("1.0.0", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *fetched != 0 {
		t.Fatalf("fetched %d times, want none", *fetched)
	}
}

// The test binary has no lerd.exe beside it, the state a WSL install is stuck in.
func TestRunUpdate_alreadyLatestStillRepairsLerdExe(t *testing.T) {
	stubLatestRelease(t)
	fetched := stubFetchRelease(t)
	prev := platform.Current.WindowsSide
	t.Cleanup(func() { platform.Current.WindowsSide = prev })
	platform.Current.WindowsSide = true

	if err := runUpdate("1.0.0", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *fetched != 1 {
		t.Fatalf("fetched %d times, want once", *fetched)
	}
}

// ── VersionGreaterThan pre-release ──────────────────────────────────────────

func TestVersionGreaterThan_prerelease(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.5.0", "1.5.0-beta.1", true},
		{"1.5.0-beta.1", "1.5.0", false},
		{"1.5.0-beta.2", "1.5.0-beta.1", true},
		{"1.5.0-rc.1", "1.5.0-beta.1", true},
		{"1.5.0-beta.1", "1.5.0-rc.1", false},
		{"1.5.0-beta.1", "1.5.0-beta.1", false},
		{"2.0.0-beta.1", "1.5.0", true},
		{"1.4.0", "1.5.0-beta.1", false},
		// Existing stable comparisons still work.
		{"1.5.0", "1.4.0", true},
		{"1.4.0", "1.5.0", false},
		{"1.5.0", "1.5.0", false},
	}
	for _, c := range cases {
		got := lerdUpdate.VersionGreaterThan(c.a, c.b)
		if got != c.want {
			t.Errorf("VersionGreaterThan(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// ── stripGitDescribe ────────────────────────────────────────────────────────

func TestStripGitDescribe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"1.5.0", "1.5.0"},
		{"1.5.0-dirty", "1.5.0"},
		{"1.5.0-5-gabcdef0", "1.5.0"},
		{"1.5.0-dirty", "1.5.0"},
		{"1.5.0-beta.1", "1.5.0-beta.1"},
		{"1.5.0-rc.1", "1.5.0-rc.1"},
		{"1.5.0-beta.1-dirty", "1.5.0-beta.1"},
		{"1.5.0-beta.1-3-g1a2b3c4", "1.5.0-beta.1"},
	}
	for _, c := range cases {
		got := lerdUpdate.StripGitDescribe(c.in)
		if got != c.want {
			t.Errorf("StripGitDescribe(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── FetchLatestPrerelease ───────────────────────────────────────────────────

func TestFetchLatestPrerelease_success(t *testing.T) {
	releases := []lerdUpdate.GithubReleaseForTest{
		{TagName: "v1.5.0-beta.1", Prerelease: true, Draft: false},
		{TagName: "v1.4.0", Prerelease: false, Draft: false},
	}
	body, _ := json.Marshal(releases)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	orig := lerdUpdate.APIBaseURLs
	lerdUpdate.APIBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.APIBaseURLs = orig }()

	got, err := lerdUpdate.FetchLatestPrerelease()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.5.0-beta.1" {
		t.Errorf("got %q, want v1.5.0-beta.1", got)
	}
}

func TestFetchLatestPrerelease_noPrerelease(t *testing.T) {
	releases := []lerdUpdate.GithubReleaseForTest{
		{TagName: "v1.4.0", Prerelease: false, Draft: false},
	}
	body, _ := json.Marshal(releases)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	orig := lerdUpdate.APIBaseURLs
	lerdUpdate.APIBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.APIBaseURLs = orig }()

	_, err := lerdUpdate.FetchLatestPrerelease()
	if err == nil {
		t.Fatal("expected error when no pre-release available, got nil")
	}
}

func TestFetchLatestPrerelease_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	orig := lerdUpdate.APIBaseURLs
	lerdUpdate.APIBaseURLs = func() []string { return []string{srv.URL} }
	defer func() { lerdUpdate.APIBaseURLs = orig }()

	_, err := lerdUpdate.FetchLatestPrerelease()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// ── backupBinary ────────────────────────────────────────────────────────────

func TestBackupBinary(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	// Create a data dir for the backup files.
	dataDir := filepath.Join(tmp, "lerd")
	os.MkdirAll(dataDir, 0755)

	// Create a fake binary in a separate subdirectory to avoid collision.
	binDir := filepath.Join(tmp, "bin")
	os.MkdirAll(binDir, 0755)
	fakeBin := filepath.Join(binDir, "lerd")
	os.WriteFile(fakeBin, []byte("fake-binary"), 0755)

	backupBinary(fakeBin, "1.4.0")

	// Check backup file exists.
	bakData, err := os.ReadFile(filepath.Join(dataDir, "lerd.bak"))
	if err != nil {
		t.Fatalf("backup binary not created: %v", err)
	}
	if string(bakData) != "fake-binary" {
		t.Errorf("backup content = %q, want %q", bakData, "fake-binary")
	}

	// Check version file.
	verData, err := os.ReadFile(filepath.Join(dataDir, "rollback-version"))
	if err != nil {
		t.Fatalf("rollback-version not created: %v", err)
	}
	if string(verData) != "1.4.0" {
		t.Errorf("version = %q, want %q", verData, "1.4.0")
	}
}

// ── rollback ────────────────────────────────────────────────────────────────

func TestRunRollback_noBackup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, "lerd"), 0755)

	err := runRollback()
	if err == nil {
		t.Fatal("expected error when no backup exists, got nil")
	}
}

// ── mutual exclusivity ─────────────────────────────────────────────────────

func TestUpdateCmd_mutuallyExclusive(t *testing.T) {
	cmd := NewUpdateCmd("1.0.0")
	cmd.SetArgs([]string{"--beta", "--rollback"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --beta and --rollback are set, got nil")
	}
}

// ── stripTypeNotify ────────────────────────────────────────────────────────

func TestStripTypeNotify_removesLineWithSurroundingContext(t *testing.T) {
	in := "[Service]\nType=notify\nExecStart=/bin/x\nRestart=on-failure\n"
	want := "[Service]\nExecStart=/bin/x\nRestart=on-failure\n"
	if got := stripTypeNotify(in); got != want {
		t.Errorf("stripTypeNotify:\n got %q\nwant %q", got, want)
	}
}

func TestStripTypeNotify_ignoresWhitespaceVariants(t *testing.T) {
	in := "[Service]\n  Type=notify  \nExecStart=/bin/x\n"
	want := "[Service]\nExecStart=/bin/x\n"
	if got := stripTypeNotify(in); got != want {
		t.Errorf("stripTypeNotify did not strip whitespace-padded line:\n got %q\nwant %q", got, want)
	}
}

func TestStripTypeNotify_doesNotTouchOtherDirectives(t *testing.T) {
	in := "[Service]\nType=simple\nExecStart=/bin/x\n# Type=notify in a comment\n"
	if got := stripTypeNotify(in); got != in {
		t.Errorf("stripTypeNotify changed unrelated content:\n got %q\nwant %q", got, in)
	}
}

func TestStripTypeNotify_noOpWhenAbsent(t *testing.T) {
	in := "[Service]\nExecStart=/bin/x\n"
	if got := stripTypeNotify(in); got != in {
		t.Errorf("stripTypeNotify changed input without Type=notify:\n got %q\nwant %q", got, in)
	}
}

// ── prepUserUnitsForRollback ───────────────────────────────────────────────

func TestPrepUserUnitsForRollback_rewritesOnlyChangedFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "systemd", "user")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	withNotify := "[Service]\nType=notify\nExecStart=/bin/x\n"
	withoutNotify := "[Service]\nExecStart=/bin/y\n"

	uiPath := filepath.Join(dir, "lerd-ui.service")
	watcherPath := filepath.Join(dir, "lerd-watcher.service")
	if err := os.WriteFile(uiPath, []byte(withNotify), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(watcherPath, []byte(withoutNotify), 0644); err != nil {
		t.Fatal(err)
	}

	prepUserUnitsForRollback("lerd-ui.service", "lerd-watcher.service")

	gotUI, _ := os.ReadFile(uiPath)
	if string(gotUI) != "[Service]\nExecStart=/bin/x\n" {
		t.Errorf("lerd-ui.service was not stripped: %q", gotUI)
	}
	gotWatcher, _ := os.ReadFile(watcherPath)
	if string(gotWatcher) != withoutNotify {
		t.Errorf("lerd-watcher.service must not be rewritten when unchanged: %q", gotWatcher)
	}
}

func TestPrepUserUnitsForRollback_skipsMissingFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	prepUserUnitsForRollback("lerd-ui.service")
}

// lerd.exe rides in every Linux archive, but only a WSL install uses it.
func TestCompanionBinaries_lerdExeOnlyOnWSL(t *testing.T) {
	prev := platform.Current.WindowsSide
	t.Cleanup(func() { platform.Current.WindowsSide = prev })
	platform.Current.WindowsSide = false
	if got := companionBinaries(); !reflect.DeepEqual(got, []string{"lerd-tray"}) {
		t.Errorf("off WSL got %v, want the tray only", got)
	}
	platform.Current.WindowsSide = true
	if got := companionBinaries(); !reflect.DeepEqual(got, []string{"lerd-tray", "lerd.exe"}) {
		t.Errorf("on WSL got %v, want the tray and lerd.exe", got)
	}
}

// restoreFixture is a WSL install at self whose release fetch serves a directory
// holding the given files, and counts how often it was fetched.
func restoreFixture(t *testing.T, self string, archived ...string) *int {
	t.Helper()
	prevWSL, prevFetch := platform.Current.WindowsSide, fetchRelease
	t.Cleanup(func() { platform.Current.WindowsSide, fetchRelease = prevWSL, prevFetch })
	platform.Current.WindowsSide = true
	if err := os.WriteFile(self, []byte("lerd"), 0o755); err != nil {
		t.Fatal(err)
	}
	extracted := t.TempDir()
	for _, name := range archived {
		if err := os.WriteFile(filepath.Join(extracted, name), []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fetched := 0
	fetchRelease = func(string) (string, func(), error) {
		fetched++
		return extracted, func() {}, nil
	}
	return &fetched
}

func TestRestoreLerdExeFetchesItWhenMissing(t *testing.T) {
	self := filepath.Join(t.TempDir(), "lerd")
	fetched := restoreFixture(t, self, "lerd", "lerd.exe")

	if err := restoreLerdExe(self, "v1.38.0"); err != nil {
		t.Fatal(err)
	}
	if *fetched != 1 {
		t.Fatalf("fetched %d times, want once", *fetched)
	}
	got, err := os.ReadFile(filepath.Join(filepath.Dir(self), "lerd.exe"))
	if err != nil || string(got) != "lerd.exe" {
		t.Fatalf("lerd.exe = %q, %v", got, err)
	}
}

func TestRestoreLerdExeLeavesAPresentOneAlone(t *testing.T) {
	self := filepath.Join(t.TempDir(), "lerd")
	fetched := restoreFixture(t, self, "lerd", "lerd.exe")
	exe := filepath.Join(filepath.Dir(self), "lerd.exe")
	if err := os.WriteFile(exe, []byte("mine"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := restoreLerdExe(self, "v1.38.0"); err != nil {
		t.Fatal(err)
	}
	if *fetched != 0 {
		t.Fatalf("fetched %d times, want none", *fetched)
	}
	if got, _ := os.ReadFile(exe); string(got) != "mine" {
		t.Fatalf("lerd.exe overwritten with %q", got)
	}
}

func TestRestoreLerdExeDoesNothingOffWSL(t *testing.T) {
	self := filepath.Join(t.TempDir(), "lerd")
	fetched := restoreFixture(t, self, "lerd", "lerd.exe")
	platform.Current.WindowsSide = false

	if err := restoreLerdExe(self, "v1.38.0"); err != nil {
		t.Fatal(err)
	}
	if *fetched != 0 {
		t.Fatalf("fetched %d times, want none", *fetched)
	}
}

func TestRestoreLerdExeReportsAReleaseWithoutIt(t *testing.T) {
	self := filepath.Join(t.TempDir(), "lerd")
	restoreFixture(t, self, "lerd")

	err := restoreLerdExe(self, "v1.38.0")
	if err == nil || !strings.Contains(err.Error(), "lerd.exe") {
		t.Fatalf("restoreLerdExe() = %v, want an error naming lerd.exe", err)
	}
}

func TestRestoreLerdExeReportsAFailedDownload(t *testing.T) {
	self := filepath.Join(t.TempDir(), "lerd")
	restoreFixture(t, self, "lerd")
	fetchRelease = func(string) (string, func(), error) { return "", func() {}, errors.New("network down") }

	err := restoreLerdExe(self, "v1.38.0")
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("restoreLerdExe() = %v, want the download error", err)
	}
}

// A package manager owns that directory, so a download could only fail or be
// wiped by the next upgrade: name the problem instead of fetching.
func TestRestoreLerdExeRefusesAPackagedInstall(t *testing.T) {
	dir := t.TempDir()
	fetched := restoreFixture(t, filepath.Join(dir, "lerd"), "lerd", "lerd.exe")

	// The Nix prefix, because the /usr ones only count as packaged on Linux.
	packaged := "/nix/store/abc-lerd/bin/lerd"
	err := restoreLerdExe(packaged, "v1.38.0")
	if err == nil || !strings.Contains(err.Error(), packaged) {
		t.Fatalf("restoreLerdExe() = %v, want an error naming the packaged binary", err)
	}
	if *fetched != 0 {
		t.Fatalf("fetched %d times, want none", *fetched)
	}
}

// An older lerd.exe already installed must not hide a release that dropped it.
func TestReleaseLacksLerdExeLooksAtTheArchive(t *testing.T) {
	self := filepath.Join(t.TempDir(), "lerd")
	restoreFixture(t, self)
	if err := os.WriteFile(filepath.Join(filepath.Dir(self), "lerd.exe"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	extracted := t.TempDir()

	if !releaseLacksLerdExe(extracted) {
		t.Fatal("want the release reported as lacking lerd.exe")
	}
	if err := os.WriteFile(filepath.Join(extracted, "lerd.exe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if releaseLacksLerdExe(extracted) {
		t.Fatal("want a release carrying lerd.exe accepted")
	}
	platform.Current.WindowsSide = false
	if err := os.Remove(filepath.Join(extracted, "lerd.exe")); err != nil {
		t.Fatal(err)
	}
	if releaseLacksLerdExe(extracted) {
		t.Fatal("off WSL lerd.exe is not expected in the release")
	}
}

func TestReplaceCompanionSkipsWhatTheArchiveLacks(t *testing.T) {
	dir := t.TempDir()
	if err := replaceCompanion(filepath.Join(dir, "absent"), filepath.Join(dir, "dst")); err != nil {
		t.Fatalf("replaceCompanion() = %v, want nil for an archive without it", err)
	}
}

func TestReplaceCompanionReportsAFailedCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceCompanion(src, filepath.Join(dir, "no-such-dir", "dst")); err == nil {
		t.Fatal("replaceCompanion() = nil, want the copy error")
	}
}
