package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/composer"
	"github.com/geodro/lerd/internal/config"
	"github.com/spf13/cobra"
)

// NewComposerCmd returns the composer command. It runs `php composer.phar`
// inside the project's FPM container (so composer always has the matching
// PHP runtime) and, after the command exits, syncs `composer global` binaries
// from `$COMPOSER_HOME/vendor/bin/` into lerd's bin dir as wrapper scripts,
// so globally required packages like psy/psysh or laravel/installer become
// callable from the host shell on every supported platform.
func NewComposerCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "composer [args...]",
		Short:              "Run composer in the project's container, syncing composer-global bins onto PATH",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runComposer(args)
		},
	}
}

func runComposer(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	composerPhar := filepath.Join(config.BinDir(), "composer.phar")
	phpArgs := append([]string{composerPhar}, args...)
	code, runErr := RunPHPCaptureEnv(cwd, phpArgs, []string{composer.ProcessTimeoutEnv()})

	// Sync regardless of composer exit status, so a `composer global remove`
	// that fails partway still cleans up wrappers whose source bin is gone.
	lerdBin, _ := os.Executable()
	if lerdBin == "" {
		home, _ := os.UserHomeDir()
		lerdBin = filepath.Join(home, ".local", "bin", "lerd")
	}
	if syncErr := syncComposerGlobalBins(composerGlobalBinDir(), config.BinDir(), lerdBin); syncErr != nil {
		fmt.Fprintf(os.Stderr, "lerd: warning: failed to sync composer global wrappers: %v\n", syncErr)
	}

	if runErr != nil {
		return runErr
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// composerGlobalBinDir resolves the directory where composer drops binaries
// for globally required packages, honouring COMPOSER_HOME and XDG.
func composerGlobalBinDir() string {
	if v := os.Getenv("COMPOSER_HOME"); v != "" {
		return filepath.Join(v, "vendor", "bin")
	}
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "composer", "vendor", "bin")
}
