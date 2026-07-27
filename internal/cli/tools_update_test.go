package cli

import (
	"archive/zip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/tools"
)

// testManifest builds a manifest whose URLs point at the test server, so
// updateTool exercises the real download-swap-stamp flow without the network.
func testManifest(srvURL string) *tools.Manifest {
	return &tools.Manifest{Tools: map[string]tools.Tool{
		"composer": {Version: "9.9.9", URL: srvURL + "/composer.phar"},
		"mkcert":   {Version: "v9.9.9", URL: srvURL + "/mkcert"},
		"fnm":      {Version: "v9.9.9", URL: srvURL + "/fnm.zip"},
	}}
}

func TestUpdateTool_SwapsBinaryAndStamps(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "new-phar")
	}))
	defer srv.Close()

	pharPath := filepath.Join(config.BinDir(), "composer.phar")
	if err := os.WriteFile(pharPath, []byte("old-phar"), 0o755); err != nil {
		t.Fatal(err)
	}

	pins := pinnedTools{m: testManifest(srv.URL)}
	if err := updateTool(&pins, "composer"); err != nil {
		t.Fatalf("updateTool: %v", err)
	}
	got, _ := os.ReadFile(pharPath)
	if string(got) != "new-phar" {
		t.Errorf("composer.phar = %q, want the downloaded build", got)
	}
	if v := tools.InstalledVersion("composer"); v != "9.9.9" {
		t.Errorf("stamped version = %q, want 9.9.9", v)
	}
	if _, err := os.Stat(pharPath + ".new"); !os.IsNotExist(err) {
		t.Error("temp .new file left behind")
	}
}

func TestUpdateTool_FailedDownloadKeepsOldBinary(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	mkcertPath := filepath.Join(config.BinDir(), "mkcert")
	if err := os.WriteFile(mkcertPath, []byte("old-mkcert"), 0o755); err != nil {
		t.Fatal(err)
	}

	pins := pinnedTools{m: testManifest(srv.URL)}
	if err := updateTool(&pins, "mkcert"); err == nil {
		t.Fatal("expected error for failed download")
	}
	got, _ := os.ReadFile(mkcertPath)
	if string(got) != "old-mkcert" {
		t.Errorf("mkcert = %q, want the old binary untouched", got)
	}
}

func TestUpdateTool_FnmExtractsAndStamps(t *testing.T) {
	if _, err := exec.LookPath("unzip"); err != nil {
		t.Skip("unzip not available")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	var zipBuf []byte
	{
		f, err := os.CreateTemp(t.TempDir(), "fnm-*.zip")
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		// A zero Modified extracts as a nonsense future mtime, which would
		// (correctly) invalidate the fresh version stamp.
		entry, err := zw.CreateHeader(&zip.FileHeader{Name: "fnm", Modified: time.Now().Add(-time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(entry, "new-fnm")
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		f.Close()
		zipBuf, err = os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipBuf)
	}))
	defer srv.Close()

	if err := os.WriteFile(filepath.Join(config.BinDir(), "fnm"), []byte("old-fnm"), 0o755); err != nil {
		t.Fatal(err)
	}

	pins := pinnedTools{m: testManifest(srv.URL)}
	if err := updateTool(&pins, "fnm"); err != nil {
		t.Fatalf("updateTool: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(config.BinDir(), "fnm"))
	if string(got) != "new-fnm" {
		t.Errorf("fnm = %q, want the extracted build", got)
	}
	if v := tools.InstalledVersion("fnm"); v != "v9.9.9" {
		t.Errorf("stamped version = %q, want v9.9.9", v)
	}
	if _, err := os.Stat(filepath.Join(config.BinDir(), "fnm.zip")); !os.IsNotExist(err) {
		t.Error("fnm.zip left behind")
	}
}
