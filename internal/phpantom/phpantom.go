// Package phpantom manages the phpantom_lsp PHP language server binary that
// powers tinker autocomplete, diagnostics, and hover in the web UI.
//
// phpantom_lsp (https://github.com/PHPantom-dev/phpantom_lsp) is a single,
// self-contained Rust binary: it bundles phpstorm-stubs and the Mago parser
// and needs no PHP runtime to analyze a project. That lets lerd run it on the
// host pointed at the project directory, alongside the other host tools it
// already manages (fnm, mkcert, composer) in BinDir, rather than baking it
// into the per-version PHP container images.
package phpantom

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/download"
)

// Version pins the phpantom_lsp release lerd installs. Bump alongside a
// tested upgrade; the binary is re-fetched when the on-disk copy is missing.
const Version = "0.10.0"

// binName is the executable's name, both inside the release archive and in
// BinDir: the Windows build ships phpantom_lsp.exe.
func binName() string { return config.ExeName("phpantom_lsp") }

// BinPath is the managed location of the phpantom_lsp executable.
func BinPath() string {
	return filepath.Join(config.BinDir(), binName())
}

// stampPath is the sidecar that records which Version the on-disk binary is, so
// bumping Version re-fetches instead of silently reusing the stale binary.
func stampPath() string {
	return BinPath() + ".version"
}

// Installed reports whether the managed binary is present and matches the
// pinned Version. A bare binary with no (or a mismatched) stamp counts as not
// installed so EnsureBinary upgrades it.
func Installed() bool {
	info, err := os.Stat(BinPath())
	if err != nil || info.IsDir() {
		return false
	}
	stamp, err := os.ReadFile(stampPath())
	return err == nil && strings.TrimSpace(string(stamp)) == Version
}

// assetName returns the release archive name for the host platform.
func assetName() (string, error) {
	return assetFor(runtime.GOOS, runtime.GOARCH)
}

// assetFor maps a platform to its release archive: a tarball on Linux and
// macOS, a zip on Windows.
func assetFor(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "phpantom_lsp-x86_64-unknown-linux-gnu.tar.gz", nil
	case "linux/arm64":
		return "phpantom_lsp-aarch64-unknown-linux-gnu.tar.gz", nil
	case "darwin/amd64":
		return "phpantom_lsp-x86_64-apple-darwin.tar.gz", nil
	case "darwin/arm64":
		return "phpantom_lsp-aarch64-apple-darwin.tar.gz", nil
	case "windows/amd64":
		return "phpantom_lsp-x86_64-pc-windows-msvc.zip", nil
	case "windows/arm64":
		return "phpantom_lsp-aarch64-pc-windows-msvc.zip", nil
	default:
		return "", fmt.Errorf("phpantom_lsp: unsupported platform %s/%s", goos, goarch)
	}
}

func downloadURL() (string, error) {
	asset, err := assetName()
	if err != nil {
		return "", err
	}
	return assetURL(asset), nil
}

func assetURL(asset string) string {
	return fmt.Sprintf("https://github.com/PHPantom-dev/phpantom_lsp/releases/download/%s/%s", Version, asset)
}

// EnsureBinary downloads and extracts phpantom_lsp into BinDir when it is not
// already present. It is safe to call on every connection: once installed it
// returns immediately. The download honours ctx, so a caller whose request is
// cancelled (e.g. the browser tab closing mid-download) aborts the fetch.
func EnsureBinary(ctx context.Context, w io.Writer) error {
	if Installed() {
		return nil
	}
	asset, err := assetName()
	if err != nil {
		return err
	}
	url := assetURL(asset)
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		return err
	}
	fmt.Fprintf(w, "Downloading phpantom_lsp %s\n", Version)

	tmp, err := os.CreateTemp("", "phpantom_lsp-*-"+asset)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	if err := download.File(ctx, url, tmpName, 0o644, io.Discard); err != nil {
		return fmt.Errorf("phpantom_lsp download: %w", err)
	}
	if strings.HasSuffix(asset, ".zip") {
		err = extractZipBinary(tmpName, BinPath())
	} else {
		var f *os.File
		if f, err = os.Open(tmpName); err != nil {
			return err
		}
		err = extractBinary(f, BinPath())
		f.Close()
	}
	if err != nil {
		return err
	}
	// Stamp the version last, so a binary is only ever considered up to date
	// once it is fully in place. A failed stamp write leaves Installed() false
	// and the next call re-fetches rather than running an unstamped binary.
	return os.WriteFile(stampPath(), []byte(Version+"\n"), 0o644)
}

// extractBinary pulls the phpantom_lsp executable out of the gzipped tar
// stream and installs it at dest via installBinary.
func extractBinary(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("phpantom_lsp: %q not found in archive", binName())
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName() {
			continue
		}
		return installBinary(tr, dest)
	}
}

// extractZipBinary is extractBinary for the zip the Windows release ships.
func extractZipBinary(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() || filepath.Base(f.Name) != binName() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return installBinary(rc, dest)
	}
	return fmt.Errorf("phpantom_lsp: %q not found in archive", binName())
}

// installBinary copies r to dest via an atomic rename. It writes to a per-call
// unique temp file in the same directory so two concurrent installs can never
// interleave writes into a shared scratch path and rename a corrupted binary
// into place; the temp is always cleaned up.
func installBinary(r io.Reader, dest string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), binName()+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed; cleans up on any failure

	if _, err := io.Copy(tmp, r); err != nil { //nolint:gosec // trusted release archive
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}
