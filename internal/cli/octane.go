package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewOctaneCmd returns the octane parent command with a reload subcommand.
func NewOctaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "octane",
		Short: "Manage Laravel Octane (FrankenPHP worker mode) for the current site",
	}
	cmd.AddCommand(newOctaneReloadCmd("reload"))
	return cmd
}

// NewOctaneReloadCmd returns the standalone octane:reload command.
func NewOctaneReloadCmd() *cobra.Command { return newOctaneReloadCmd("octane:reload") }

// newOctaneReloadCmd toggles Octane auto-reload (octane:start --watch) on or off
// for the current site. With no argument it prints the current state. When
// toggled it persists the per-project preference and, when the site serves via
// FrankenPHP worker mode, re-applies the FrankenPHP container so the new
// entrypoint takes effect immediately.
func newOctaneReloadCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " [on|off]",
		Short: "Toggle Octane auto-reload on file changes (octane:start --watch) for the current site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if !SiteHasOctane(cwd) {
				return fmt.Errorf("laravel/octane is not installed in this project\nInstall it with: composer require laravel/octane\nSee https://laravel.com/docs/13.x/octane")
			}

			if len(args) == 0 {
				state := "off"
				if config.ProjectReloadsWorker(cwd, "octane") {
					state = "on"
				}
				fmt.Printf("Octane auto-reload (octane:start --watch): %s\n", state)
				return nil
			}

			var enable bool
			switch strings.ToLower(args[0]) {
			case "on", "true", "1", "enable", "enabled":
				enable = true
			case "off", "false", "0", "disable", "disabled":
				enable = false
			default:
				return fmt.Errorf("expected 'on' or 'off', got %q", args[0])
			}

			siteName, err := queueSiteName(cwd)
			if err != nil {
				return err
			}
			phpVersion, err := phpDet.DetectVersion(cwd)
			if err != nil {
				cfg, _ := config.LoadGlobal()
				phpVersion = cfg.PHP.DefaultVersion
			}
			if err := ApplyOctaneReload(siteName, cwd, phpVersion, enable); err != nil {
				return err
			}

			feedback.Begin()
			if enable {
				feedback.Done("Octane auto-reload enabled")
				feedback.Note("Octane restarts workers on file changes")
			} else {
				feedback.Done("Octane auto-reload disabled")
			}
			return nil
		},
	}
}

// ApplyOctaneReload persists the per-project Octane auto-reload preference and,
// when the site currently serves via FrankenPHP worker mode, re-applies the
// FrankenPHP container so the new entrypoint (watch vs standard) takes effect.
// FinishFrankenPHPLink re-resolves the entrypoint from the freshly persisted
// preference and restarts the container only when the quadlet content changed.
func ApplyOctaneReload(siteName, sitePath, phpVersion string, enabled bool) error {
	// Refuse to enable when the watcher prerequisite is missing rather than
	// persisting a preference the server can't honour. Without this the toggle
	// would read "on" while ResolveFrankenPHPWorkerEntrypoint quietly ran the
	// standard command, so the displayed state would diverge from reality.
	if enabled && !projectHasChokidar(sitePath) {
		return fmt.Errorf("Octane auto-reload needs the chokidar npm package, which is not installed in this project\nInstall it with: npm install -D chokidar\n(Vite 8 no longer ships it, so a plain npm install is not enough)")
	}
	if err := config.SetProjectWorkerReload(sitePath, "octane", enabled); err != nil {
		return err
	}
	site, err := config.FindSite(siteName)
	if err != nil || site == nil {
		return nil
	}
	if !site.IsFrankenPHP() || !site.RuntimeWorker {
		// Preference saved; it takes effect once the site serves via FrankenPHP
		// worker mode (lerd runtime frankenphp --worker).
		return nil
	}
	if err := siteops.FinishFrankenPHPLink(*site); err != nil {
		return fmt.Errorf("re-applying FrankenPHP container: %w", err)
	}
	return nil
}

// SiteHasOctane returns true if composer.json lists laravel/octane as a dependency.
func SiteHasOctane(sitePath string) bool {
	data, err := os.ReadFile(filepath.Join(sitePath, "composer.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"laravel/octane"`)
}
