//go:build windows

package cli

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body)) //nolint:errcheck
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseArchiveNameIsTheWindowsZip(t *testing.T) {
	if got := releaseArchiveName("1.30.0", "arm64"); got != "lerd_1.30.0_windows_arm64.zip" {
		t.Errorf("got %q", got)
	}
}

func TestExtractReleaseArchiveUnpacksTheFlatZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "release.zip")
	writeZip(t, archive, map[string]string{"lerd.exe": "lerd", "lerd-tray.exe": "tray", "LICENSE": "mit"})
	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755) //nolint:errcheck

	if err := extractReleaseArchive(archive, out); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"lerd.exe": "lerd", "lerd-tray.exe": "tray"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestExtractReleaseArchiveRefusesAPathOutsideTheFolder(t *testing.T) {
	for _, name := range []string{"../evil.exe", `..\evil.exe`, "sub/lerd.exe"} {
		dir := t.TempDir()
		archive := filepath.Join(dir, "release.zip")
		writeZip(t, archive, map[string]string{name: "x"})
		out := filepath.Join(dir, "out")
		os.Mkdir(out, 0o755) //nolint:errcheck

		if err := extractReleaseArchive(archive, out); err == nil {
			t.Errorf("%q was extracted, want a refusal", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "evil.exe")); err == nil {
			t.Errorf("%q escaped the folder", name)
		}
	}
}

func TestSwapBinaryReplacesARunningExe(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "lerd.exe")
	ping, err := exec.LookPath("ping.exe")
	if err != nil {
		t.Skip("ping.exe not found")
	}
	if err := copyFile(ping, dest, 0o755); err != nil {
		t.Fatal(err)
	}
	running := exec.Command(dest, "-n", "30", "127.0.0.1")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { running.Process.Kill(); running.Wait() }() //nolint:errcheck

	src := filepath.Join(dir, "new.exe")
	os.WriteFile(src, []byte("new"), 0o755) //nolint:errcheck
	if err := swapBinary(src, dest); err != nil {
		t.Fatalf("swapping a running exe: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "new" {
		t.Errorf("dest = %q, want the new binary", got)
	}
	if aside, _ := filepath.Glob(dest + ".old-*"); len(aside) != 1 {
		t.Errorf("want the running copy moved aside, got %v", aside)
	}
}

func TestSwapBinaryClearsStaleCopiesAndPlacesAFreshOne(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "lerd.exe")
	stale := dest + ".old-1"
	os.WriteFile(stale, []byte("stale"), 0o755) //nolint:errcheck
	src := filepath.Join(dir, "new.exe")
	os.WriteFile(src, []byte("new"), 0o755) //nolint:errcheck

	if err := swapBinary(src, dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "new" {
		t.Errorf("dest = %q", got)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("stale copy left behind")
	}
}

func TestSwapBinaryPutsTheOldOneBackWhenTheCopyFails(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "lerd.exe")
	os.WriteFile(dest, []byte("old"), 0o755) //nolint:errcheck

	if err := swapBinary(filepath.Join(dir, "missing.exe"), dest); err == nil {
		t.Fatal("want an error for a missing source")
	}
	if got, _ := os.ReadFile(dest); string(got) != "old" {
		t.Errorf("dest = %q, want the old binary restored", got)
	}
}

func TestDownloadReleaseBinaryFetchesTheWindowsZip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "release.zip")
	writeZip(t, archive, map[string]string{"lerd.exe": "lerd", "lerd-tray.exe": "tray"})
	body, _ := os.ReadFile(archive)

	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write(body) //nolint:errcheck
	}))
	defer srv.Close()
	orig := githubDownloadBases
	githubDownloadBases = func() []string { return []string{srv.URL} }
	defer func() { githubDownloadBases = orig }()

	dir, cleanup, err := downloadReleaseBinary("v1.30.0")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if want := "/v1.30.0/lerd_1.30.0_windows_" + runtime.GOARCH + ".zip"; path != want {
		t.Errorf("requested %q, want %q", path, want)
	}
	for _, name := range []string{"lerd.exe", "lerd-tray.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not extracted: %v", name, err)
		}
	}
}

func TestBackupBinaryKeepsTheWindowsTray(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, "lerd"), 0o755) //nolint:errcheck
	bin := filepath.Join(tmp, "bin")
	os.MkdirAll(bin, 0o755)                                                  //nolint:errcheck
	os.WriteFile(filepath.Join(bin, "lerd.exe"), []byte("lerd"), 0o755)      //nolint:errcheck
	os.WriteFile(filepath.Join(bin, "lerd-tray.exe"), []byte("tray"), 0o755) //nolint:errcheck

	backupBinary(filepath.Join(bin, "lerd.exe"), "1.30.0")

	if got, err := os.ReadFile(filepath.Join(tmp, "lerd", "lerd-tray.bak")); err != nil || !strings.Contains(string(got), "tray") {
		t.Errorf("tray backup = %q, %v", got, err)
	}
}
