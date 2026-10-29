// Package phpantom manages the phpantom_lsp language server behind tinker's
// autocomplete, diagnostics and hover. It is a self-contained Rust binary that
// needs no PHP, so it runs on the host from BinDir instead of in the PHP images.
package phpantom

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/download"
)

// Version pins the phpantom_lsp release lerd installs. Bump alongside a
// tested upgrade; the binary is re-fetched when the on-disk copy is missing.
const Version = "0.11.1"

const binName = "phpantom_lsp"

// BinPath is the managed location of the phpantom_lsp executable.
func BinPath() string {
	return filepath.Join(config.BinDir(), binName)
}

// fullTokensConfig is the global config lerd hands phpantom. Its default
// "contextual" mode assumes an editor grammar colours classes and calls, which
// Monaco's PHP grammar cannot, so tinker asks for the full token stream.
const fullTokensConfig = "[semantic_tokens]\nmode = \"full\"\n"

// Command returns a phpantom_lsp process rooted at dir. XDG_CONFIG_HOME points
// at a lerd-owned folder because a .phpantom.toml is the only way to pick the
// token mode; a project's own .phpantom.toml still overrides it.
func Command(ctx context.Context, dir string) (*exec.Cmd, error) {
	home := filepath.Join(config.DataDir(), "phpantom")
	cfgDir := filepath.Join(home, "phpantom_lsp")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}
	if err := writeConfig(cfgDir); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, BinPath())
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+home)
	return cmd, nil
}

// writeConfig replaces the config through a rename, so a phpantom started by
// another tinker tab never reads a half-written file.
func writeConfig(dir string) error {
	tmp, err := os.CreateTemp(dir, ".phpantom-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(fullTokensConfig); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, ".phpantom.toml"))
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

// assetName returns the release tarball name for the host platform.
func assetName() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "phpantom_lsp-x86_64-unknown-linux-gnu.tar.gz", nil
	case "linux/arm64":
		return "phpantom_lsp-aarch64-unknown-linux-gnu.tar.gz", nil
	case "darwin/amd64":
		return "phpantom_lsp-x86_64-apple-darwin.tar.gz", nil
	case "darwin/arm64":
		return "phpantom_lsp-aarch64-apple-darwin.tar.gz", nil
	default:
		return "", fmt.Errorf("phpantom_lsp: unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func downloadURL() (string, error) {
	asset, err := assetName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/PHPantom-dev/phpantom_lsp/releases/download/%s/%s", Version, asset), nil
}

// EnsureBinary downloads and extracts phpantom_lsp into BinDir when it is not
// already present. It is safe to call on every connection: once installed it
// returns immediately. The download honours ctx, so a caller whose request is
// cancelled (e.g. the browser tab closing mid-download) aborts the fetch.
func EnsureBinary(ctx context.Context, w io.Writer) error {
	if Installed() {
		return nil
	}
	url, err := downloadURL()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		return err
	}
	fmt.Fprintf(w, "Downloading phpantom_lsp %s\n", Version)

	tmp, err := os.CreateTemp("", "phpantom_lsp-*.tar.gz")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	if err := download.File(ctx, url, tmpName, 0o644, io.Discard); err != nil {
		return fmt.Errorf("phpantom_lsp download: %w", err)
	}
	f, err := os.Open(tmpName)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := extractBinary(f, BinPath()); err != nil {
		return err
	}
	// Stamp the version last, so a binary is only ever considered up to date
	// once it is fully in place. A failed stamp write leaves Installed() false
	// and the next call re-fetches rather than running an unstamped binary.
	return os.WriteFile(stampPath(), []byte(Version+"\n"), 0o644)
}

// extractBinary pulls the phpantom_lsp executable out of the gzipped tar
// stream and installs it at dest via an atomic rename. It extracts to a
// per-call unique temp file in the same directory so two concurrent installs
// can never interleave writes into a shared scratch path and rename a
// corrupted binary into place; the temp is always cleaned up.
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
			return fmt.Errorf("phpantom_lsp: %q not found in archive", binName)
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName {
			continue
		}
		tmp, err := os.CreateTemp(filepath.Dir(dest), binName+"-*.tmp")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName) // no-op once renamed; cleans up on any failure

		if _, err := io.Copy(tmp, tr); err != nil { //nolint:gosec // trusted release archive
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
}
