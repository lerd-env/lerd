package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	phpArgs := append([]string{composer.PharPath()}, args...)
	code, runErr := RunPHPCaptureEnv(cwd, phpArgs, []string{composer.ProcessTimeoutEnv()})

	// Sync regardless of composer exit status, so a `composer global remove`
	// that fails partway still cleans up wrappers whose source bin is gone.
	lerdBin := config.LerdBinary()
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

// composerHomeDir resolves composer's own home the way composer does, so its
// global auth.json and packages are the ones lerd's composer sees. It is a
// composer project like any other: the manifest and lock naming what
// `composer global require` installed live here.
func composerHomeDir() string {
	home, _ := os.UserHomeDir()
	return resolveComposerHome(os.Environ(), home, func(p string) bool {
		info, err := os.Stat(p)
		return err == nil && info.IsDir()
	})
}

// resolveComposerHome mirrors composer's Factory::getHomeDir: COMPOSER_HOME
// when set, else the first existing of the XDG directory (only on a system
// that uses XDG) and ~/.composer, else the first of those candidates.
func resolveComposerHome(environ []string, home string, isDir func(string) bool) string {
	get := func(key string) string {
		for _, e := range environ {
			if k, v, ok := strings.Cut(e, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
	if v := get("COMPOSER_HOME"); v != "" {
		return v
	}
	var dirs []string
	if composerUsesXDG(environ, isDir) {
		cfg := get("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(home, ".config")
		}
		dirs = append(dirs, filepath.Join(cfg, "composer"))
	}
	dirs = append(dirs, filepath.Join(home, ".composer"))
	for _, d := range dirs {
		if isDir(d) {
			return d
		}
	}
	return dirs[0]
}

// composerUsesXDG is composer's own test: any XDG_ variable set, or /etc/xdg
// on disk.
func composerUsesXDG(environ []string, isDir func(string) bool) bool {
	for _, e := range environ {
		if strings.HasPrefix(e, "XDG_") {
			return true
		}
	}
	return isDir("/etc/xdg")
}

// composerGlobalBinDir resolves the directory where composer drops binaries
// for globally required packages.
func composerGlobalBinDir() string {
	return filepath.Join(composerHomeDir(), "vendor", "bin")
}

// composerHomeDirs lists every directory composer may be using as its home on
// this machine, not just the one lerd would pick.
//
// composer only takes the XDG location when something asks it to: an XDG_
// variable in the environment, or /etc/xdg on disk. With neither, which is the
// normal case on macOS, it uses ~/.composer. A machine can therefore end up
// with global packages in both, installed by composer runs that disagreed, and
// looking in one of them finds a tool that is really in the other.
func composerHomeDirs() []string {
	if v := os.Getenv("COMPOSER_HOME"); v != "" {
		return []string{v}
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return []string{composerHomeDir()}
	}
	dirs := []string{composerHomeDir()}
	if legacy := filepath.Join(home, ".composer"); legacy != dirs[0] {
		dirs = append(dirs, legacy)
	}
	return dirs
}
