//go:build darwin || windows

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/geodro/lerd/internal/certs"
	"github.com/geodro/lerd/internal/composer"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/phpantom"
	"github.com/geodro/lerd/internal/tools"
)

func downloadBinaries(w io.Writer) error {
	var pins pinnedTools

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
	case cfg != nil && cfg.NodeManager() == "fnm":
		if err := ensureFnmBinary(w); err != nil {
			return err
		}
	default:
		if err := ensureMiseBinary(w); err != nil {
			if !errors.Is(err, tools.ErrNoAsset) {
				return err
			}
			feedback.WarnOn(w, "%v; Node versions are not managed here, the system Node is used", err)
		} else {
			removeFnmBinary()
		}
	}

	// mkcert. A platform with no build only loses locally trusted HTTPS.
	mkcertPath := certs.MkcertPath()
	if _, err := os.Stat(mkcertPath); os.IsNotExist(err) {
		if err := replaceTool(&pins, "mkcert", mkcertPath, w); err != nil {
			if !errors.Is(err, tools.ErrNoAsset) {
				return fmt.Errorf("mkcert download: %w", err)
			}
			feedback.WarnOn(w, "%v; HTTPS certificates are unavailable, use the .localhost mode", err)
		}
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
