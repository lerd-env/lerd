package phpantom

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssetName_KnownPlatforms(t *testing.T) {
	// assetName reads runtime.GOOS/GOARCH; just assert the current platform
	// resolves to a plausible release asset (or a clear unsupported error).
	name, err := assetName()
	if err != nil {
		if !strings.Contains(err.Error(), "unsupported platform") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if !strings.HasPrefix(name, "phpantom_lsp-") || !(strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip")) {
		t.Fatalf("asset name %q does not look like a release archive", name)
	}
}

func TestAssetFor_Windows(t *testing.T) {
	cases := map[string]string{
		"amd64": "phpantom_lsp-x86_64-pc-windows-msvc.zip",
		"arm64": "phpantom_lsp-aarch64-pc-windows-msvc.zip",
	}
	for arch, want := range cases {
		got, err := assetFor("windows", arch)
		if err != nil || got != want {
			t.Errorf("assetFor(windows, %s) = %q, %v; want %q", arch, got, err, want)
		}
	}
	if _, err := assetFor("windows", "386"); err == nil {
		t.Error("assetFor(windows, 386) should be unsupported")
	}
}

func TestDownloadURL_PinsVersion(t *testing.T) {
	url, err := downloadURL()
	if err != nil {
		return // unsupported platform; covered elsewhere
	}
	if !strings.Contains(url, "/releases/download/"+Version+"/") {
		t.Fatalf("download URL %q does not pin version %q", url, Version)
	}
}

func TestExtractBinary_PullsNamedEntry(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// A decoy entry plus the real binary, to prove we select by name.
	writeTar(t, tw, "README.md", []byte("docs"))
	writeTar(t, tw, binName(), []byte("#!/binary\x00payload"))
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), binName())
	if err := extractBinary(&buf, dest); err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/binary\x00payload" {
		t.Fatalf("extracted contents = %q", got)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no exec bit: a .exe runs by its extension.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("extracted binary is not executable: %v", info.Mode())
	}
}

func TestExtractBinary_MissingEntry(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writeTar(t, tw, "something-else", []byte("x"))
	tw.Close()
	gz.Close()

	dest := filepath.Join(t.TempDir(), binName())
	if err := extractBinary(&buf, dest); err == nil {
		t.Fatal("expected error when archive lacks the binary")
	}
}

func TestExtractZipBinary_PullsNamedEntry(t *testing.T) {
	dir := t.TempDir()
	archive := writeZip(t, dir, map[string]string{
		"README.md": "docs",
		binName():   "MZ\x00payload",
	})

	dest := filepath.Join(dir, "out", binName())
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractZipBinary(archive, dest); err != nil {
		t.Fatalf("extractZipBinary: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MZ\x00payload" {
		t.Fatalf("extracted contents = %q", got)
	}
}

func TestExtractZipBinary_MissingEntry(t *testing.T) {
	dir := t.TempDir()
	archive := writeZip(t, dir, map[string]string{"something-else": "x"})
	if err := extractZipBinary(archive, filepath.Join(dir, binName())); err == nil {
		t.Fatal("expected error when archive lacks the binary")
	}
}

func TestInstalled_RequiresMatchingVersionStamp(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(BinPath()), 0o755); err != nil {
		t.Fatal(err)
	}

	if Installed() {
		t.Fatal("Installed() true with no binary present")
	}

	// A binary with no stamp counts as not installed, so a bump re-fetches.
	if err := os.WriteFile(BinPath(), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if Installed() {
		t.Fatal("Installed() true for an unstamped binary")
	}

	// A stale stamp likewise forces a re-fetch.
	if err := os.WriteFile(stampPath(), []byte("0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Installed() {
		t.Fatal("Installed() true for a mismatched version stamp")
	}

	// Only a stamp matching the pinned Version counts as installed.
	if err := os.WriteFile(stampPath(), []byte(Version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Installed() {
		t.Fatal("Installed() false for a matching version stamp")
	}
}

func writeTar(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, "archive.zip")
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
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
