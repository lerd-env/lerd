//go:build linux

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/phpantom"
)

func downloadBinaries(w io.Writer) error {
	arch := runtime.GOARCH
	binDir := config.BinDir()

	// composer
	composerPharPath := filepath.Join(binDir, "composer.phar")
	if _, err := os.Stat(composerPharPath); os.IsNotExist(err) {
		if err := downloadFile("https://getcomposer.org/composer-stable.phar", composerPharPath, 0755, w); err != nil {
			return fmt.Errorf("composer download: %w", err)
		}
	}

	// fnm — skipped when the user drives Node via their own nvm, since lerd never
	// provisions nvm and fnm would sit unused.
	// Switching back with `lerd node:manager fnm` calls ensureFnmBinary on demand.
	cfg, _ := config.LoadGlobal()
	if cfg == nil || cfg.NodeManager() != "nvm" {
		if err := ensureFnmBinary(w); err != nil {
			return err
		}
	}

	// mkcert
	mkcertPath := certs.MkcertPath()
	if _, err := os.Stat(mkcertPath); os.IsNotExist(err) {
		mkcertArch := "amd64"
		if arch == "arm64" {
			mkcertArch = "arm64"
		}
		mkcertURL := fmt.Sprintf(
			"https://github.com/FiloSottile/mkcert/releases/latest/download/mkcert-v1.4.4-linux-%s",
			mkcertArch,
		)
		if err := downloadFile(mkcertURL, mkcertPath, 0755, w); err != nil {
			return fmt.Errorf("mkcert download: %w", err)
		}
	}

	// phpantom_lsp powers tinker autocomplete in the web UI. Best-effort:
	// the UI also fetches it lazily on first tinker connect, so a failure
	// here (offline install, unsupported arch) must not abort setup.
	if !phpantom.Installed() {
		if err := phpantom.EnsureBinary(context.Background(), w); err != nil {
			fmt.Fprintf(w, "      Warning: phpantom_lsp download failed (%v); tinker autocomplete loads on first use instead\n", err)
		}
	}

	return nil
}

// ensureFnmBinary downloads and extracts fnm into BinDir when it is missing.
// Called from downloadBinaries on a normal (fnm) install, and on demand when
// switching back to fnm with `lerd node:manager fnm` after an nvm-only setup.
func ensureFnmBinary(w io.Writer) error {
	binDir := config.BinDir()
	fnmPath := filepath.Join(binDir, "fnm")
	if _, err := os.Stat(fnmPath); err == nil {
		return nil
	}
	fnmAsset := "fnm-linux.zip"
	if runtime.GOARCH == "arm64" {
		fnmAsset = "fnm-arm64.zip"
	}
	fnmZip := filepath.Join(binDir, fnmAsset)
	if err := downloadFile(
		"https://github.com/Schniz/fnm/releases/latest/download/"+fnmAsset,
		fnmZip, 0644, w,
	); err != nil {
		return fmt.Errorf("fnm download: %w", err)
	}
	extractCmd := exec.Command("unzip", "-o", fnmZip, "fnm", "-d", binDir)
	extractCmd.Stdout = w
	extractCmd.Stderr = w
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("fnm extract: %w", err)
	}
	os.Remove(fnmZip)
	os.Chmod(fnmPath, 0755) //nolint:errcheck
	return nil
}

// ensurePortForwarding is a no-op on Linux; ensureUnprivilegedPorts handles
// port 80/443 access via the ip_unprivileged_port_start sysctl.
func ensurePortForwarding() error { return nil }
