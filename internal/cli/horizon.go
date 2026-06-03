package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewHorizonCmd returns the horizon parent command with start/stop subcommands.
func NewHorizonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "horizon",
		Short: "Manage Laravel Horizon for the current site",
	}
	cmd.AddCommand(newHorizonStartCmd("start"))
	cmd.AddCommand(newHorizonStopCmd("stop"))
	cmd.AddCommand(newHorizonReloadCmd("reload"))
	return cmd
}

// NewHorizonStartCmd returns the standalone horizon:start command.
func NewHorizonStartCmd() *cobra.Command { return newHorizonStartCmd("horizon:start") }

// NewHorizonStopCmd returns the standalone horizon:stop command.
func NewHorizonStopCmd() *cobra.Command { return newHorizonStopCmd("horizon:stop") }

// NewHorizonReloadCmd returns the standalone horizon:reload command.
func NewHorizonReloadCmd() *cobra.Command { return newHorizonReloadCmd("horizon:reload") }

// newHorizonReloadCmd toggles auto-reload mode (`horizon:listen`) on or off. With
// no argument it prints the current state. When toggled it persists the global
// setting and, if a Horizon worker is running for the current site, restarts it
// so the new command takes effect immediately.
func newHorizonReloadCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " [on|off]",
		Short: "Toggle Horizon auto-reload on file changes (horizon:listen)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}

			if len(args) == 0 {
				state := "off"
				if cfg.HorizonReloadEnabled() {
					state = "on"
				}
				fmt.Printf("Horizon auto-reload (horizon:listen): %s\n", state)
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

			if err := ApplyHorizonReload(enable); err != nil {
				return err
			}

			if enable {
				fmt.Println("Horizon auto-reload enabled — Horizon will restart workers on file changes.")
			} else {
				fmt.Println("Horizon auto-reload disabled — Horizon runs in standard mode.")
			}
			return nil
		},
	}
}

// ApplyHorizonReload persists the global Horizon auto-reload toggle and restarts
// every Horizon worker that is currently running so the new command takes effect
// immediately. The setting is global, so all running Horizon workers are
// reconciled (mirrors how the worker exec-mode change applies machine-wide).
// Restart failures are non-fatal — the setting is already persisted and applies
// on the next start; only a load/save failure is returned.
func ApplyHorizonReload(enabled bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	cfg.SetHorizonReload(enabled)
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	restartRunningHorizonWorkers()
	return nil
}

// restartRunningHorizonWorkers restarts the Horizon worker of every site that
// currently has one running, so a toggle applies without manual intervention.
// Sites without a running Horizon worker are left untouched.
func restartRunningHorizonWorkers() {
	reg, err := config.LoadSites()
	if err != nil {
		fmt.Printf("[WARN] horizon reload: load sites: %v\n", err)
		return
	}
	for i := range reg.Sites {
		site := reg.Sites[i]
		if site.Paused || !SiteHasHorizon(site.Path) {
			continue
		}
		running := false
		for _, n := range CollectRunningWorkerNames(&site) {
			if n == "horizon" {
				running = true
				break
			}
		}
		if !running {
			continue
		}

		phpVersion, perr := phpDet.DetectVersion(site.Path)
		if perr != nil {
			if cfg, cerr := config.LoadGlobal(); cerr == nil {
				phpVersion = cfg.PHP.DefaultVersion
			}
		}
		if err := HorizonStopForSite(site.Name); err != nil {
			fmt.Printf("[WARN] stop horizon (%s): %v\n", site.Name, err)
		}
		if err := HorizonStartForSite(site.Name, site.Path, phpVersion); err != nil {
			fmt.Printf("[WARN] restart horizon (%s): %v\n", site.Name, err)
		}
	}
}

func newHorizonStartCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Start Laravel Horizon for the current site as a systemd service",
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if !SiteHasHorizon(cwd) {
				return fmt.Errorf("laravel/horizon is not installed in this project\nInstall it with: composer require laravel/horizon\nSee https://laravel.com/docs/13.x/horizon")
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
			if err := HorizonStartForSite(siteName, cwd, phpVersion); err != nil {
				return err
			}
			if site, err := config.FindSite(siteName); err == nil && !site.Paused {
				_ = config.SetProjectWorkers(site.Path, CollectRunningWorkerNames(site))
			}
			return nil
		},
	}
}

func newHorizonStopCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Stop Laravel Horizon for the current site",
		RunE: func(_ *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if !SiteHasHorizon(cwd) {
				return fmt.Errorf("laravel/horizon is not installed in this project\nInstall it with: composer require laravel/horizon\nSee https://laravel.com/docs/13.x/horizon")
			}
			siteName, err := queueSiteName(cwd)
			if err != nil {
				return err
			}
			if err := HorizonStopForSite(siteName); err != nil {
				return err
			}
			if site, err := config.FindSite(siteName); err == nil && !site.Paused {
				_ = config.SetProjectWorkers(site.Path, CollectRunningWorkerNames(site))
			}
			return nil
		},
	}
}

// HorizonStartForSite starts Horizon for the named site. Conflicting workers
// (defined via ConflictsWith in the framework definition) are stopped first.
func HorizonStartForSite(siteName, sitePath, phpVersion string) error {
	fw, ok := config.GetFrameworkForDir(siteFrameworkName(siteName), sitePath)
	if !ok {
		return fmt.Errorf("no framework found for site %q", siteName)
	}
	worker, ok := fw.Workers["horizon"]
	if !ok {
		return fmt.Errorf("framework %q has no worker named \"horizon\"", fw.Label)
	}
	return WorkerStartForSite(siteName, sitePath, phpVersion, "horizon", worker, true)
}

// buildHorizonUnit renders the Horizon systemd unit body. Horizon always
// uses Redis so lerd-redis is in After=/Wants= alongside the FPM container.
func buildHorizonUnit(siteName, sitePath, phpVersion string) string {
	versionShort := strings.ReplaceAll(phpVersion, ".", "")
	fpmUnit := "lerd-php" + versionShort + "-fpm"
	container := "lerd-php" + versionShort + "-fpm"

	return fmt.Sprintf(`[Unit]
Description=Lerd Horizon (%s)
After=network.target %s.service lerd-redis.service
Wants=%s.service lerd-redis.service
BindsTo=%s.service

[Service]
Type=simple
Restart=always
RestartSec=5
ExecStart=%s exec -w %s %s php artisan horizon

[Install]
WantedBy=default.target
`, siteName, fpmUnit, fpmUnit, fpmUnit, podman.PodmanBin(), sitePath, container)
}

// HorizonStopForSite stops and removes the Horizon unit for the named site.
func HorizonStopForSite(siteName string) error {
	return WorkerStopForSite(siteName, "", "horizon")
}

// SiteHasHorizon returns true if composer.json lists laravel/horizon as a dependency.
func SiteHasHorizon(sitePath string) bool {
	data, err := os.ReadFile(filepath.Join(sitePath, "composer.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"laravel/horizon"`)
}
