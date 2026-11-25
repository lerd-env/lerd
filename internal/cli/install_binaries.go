//go:build unix

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/composer"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/phpantom"
)

// downloadBinaries fetches the host tools lerd drives that are not on disk yet:
// composer, the Node version manager and phpantom_lsp.
func downloadBinaries(w io.Writer) error {
	var pins pinnedTools

	// lerd issues certificates itself now, so the mkcert an older install
	// downloaded is only an unmaintained binary left on disk.
	os.Remove(filepath.Join(config.BinDir(), "mkcert"))         //nolint:errcheck
	os.Remove(filepath.Join(config.BinDir(), "mkcert.version")) //nolint:errcheck

	// composer
	composerPharPath := composer.PharPath()
	if _, err := os.Stat(composerPharPath); os.IsNotExist(err) {
		if err := replaceTool(&pins, "composer", composerPharPath, w); err != nil {
			return fmt.Errorf("composer download: %w", err)
		}
	}

	// The Node version manager lerd drives. Skipped for nvm, which the user
	// installs themselves. mise is only fetched when the host has none, so a
	// mise they already manage stays the one in charge.
	cfg, _ := config.LoadGlobal()
	switch {
	case cfg != nil && cfg.NodeManager() == "nvm":
	case cfg != nil && cfg.NodeManager() == "fnm": // retired: kept, never fetched
	default:
		if err := ensureMiseBinary(w); err != nil {
			return err
		}
		removeFnmBinary()
	}

	// phpantom_lsp powers tinker autocomplete in the web UI. Best-effort:
	// the UI also fetches it lazily on first tinker connect, so a failure
	// here (offline install, unsupported arch) must not abort setup.
	if !phpantom.Installed() {
		if err := phpantom.EnsureBinary(context.Background(), w); err != nil {
			feedback.WarnOn(w, "phpantom_lsp download failed (%v); tinker autocomplete loads on first use instead", err)
		}
	}

	return nil
}
