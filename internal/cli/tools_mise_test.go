package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/tools"
)

// miseRelease serves a release tarball shaped like mise's, the binary at
// mise/bin/mise, and returns pins that download it.
func miseRelease(t *testing.T, version string) *pinnedTools {
	t.Helper()
	script := "#!/bin/sh\necho '" + version + " linux-x64 (2026-01-01)'\n"
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "mise/bin/mise", Mode: 0o755, Size: int64(len(script))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(buf.Bytes()) }))
	t.Cleanup(srv.Close)
	return &pinnedTools{m: &tools.Manifest{Tools: map[string]tools.Tool{"mise": {
		Version: "v" + version,
		URL:     srv.URL + "/{asset}",
		Assets:  map[string]string{runtime.GOOS + "/" + runtime.GOARCH: "mise.tar.gz"},
	}}}}
}

// The mise lerd installs is stamped, which is what makes it lerd's to bring
// back to the pin later, unlike one the user installed in the same place.
func TestInstallMise_stampsItAsLerds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")

	if err := installMise(miseRelease(t, "2026.9.12"), home, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "mise")); err != nil {
		t.Fatalf("mise not extracted: %v", err)
	}
	if v := tools.InstalledVersion("mise"); strings.TrimPrefix(v, "v") != "2026.9.12" {
		t.Fatalf("InstalledVersion(mise) = %q, want 2026.9.12 from lerd's own mise", v)
	}
}

// The dashboard's per-tool update arrives as a request, so it must not replace
// a mise the user installed themselves in the place lerd's would be.
func TestUpdateOneTool_refusesAMiseLerdDidNotInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })
	called := false
	updateToolFn = func(*pinnedTools, string) error { called = true; return nil }

	if err := UpdateOneTool("mise"); err == nil {
		t.Error("UpdateOneTool(mise) accepted a mise lerd did not install")
	}
	if called {
		t.Error("the user's mise reached the installer")
	}
}

// tools:update reaches mise through the same per-tool entry the dashboard
// uses, and puts the pinned build back over a self-updated one.
func TestUpdateTool_bringsMiseBackToThePin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	if err := installMise(miseRelease(t, "2026.10.2"), home, io.Discard); err != nil {
		t.Fatal(err)
	}

	if err := updateTool(miseRelease(t, "2026.9.12"), "mise"); err != nil {
		t.Fatalf("updateTool(mise) = %v", err)
	}
	out, err := os.ReadFile(filepath.Join(home, ".local", "bin", "mise"))
	if err != nil || !bytes.Contains(out, []byte("2026.9.12")) {
		t.Fatalf("mise on disk after the update:\n%s", out)
	}
	if v := tools.InstalledVersion("mise"); strings.TrimPrefix(v, "v") != "2026.9.12" {
		t.Fatalf("InstalledVersion(mise) = %q, want 2026.9.12", v)
	}
}
