package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpPkgCmd returns the php:pkg parent command, which manages extra Alpine
// packages baked into the PHP-FPM image's runtime stage. Unlike php:bun (a
// runtime install into a volume), these are layered into the image at build
// time and re-applied on every rebuild, like php:ext.
func NewPhpPkgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "php:pkg",
		Short: "Manage extra Alpine packages in the PHP-FPM image",
	}
	cmd.AddCommand(newPhpPkgAddCmd())
	cmd.AddCommand(newPhpPkgRemoveCmd())
	cmd.AddCommand(newPhpPkgListCmd())
	return cmd
}

// phpPkgVersion resolves the PHP version from the --php flag, cwd detection, or
// the global default.
func phpPkgVersion(flagVer string) (string, error) {
	if flagVer != "" {
		return flagVer, nil
	}
	if cwd, err := os.Getwd(); err == nil {
		if v, err := phpDet.DetectVersion(cwd); err == nil {
			return v, nil
		}
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return "", err
	}
	return cfg.PHP.DefaultVersion, nil
}

func newPhpPkgAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <package...>",
		Short: "Add Alpine packages to every PHP-FPM image",
		Long: "Adds packages to your declared set, which applies to every PHP image lerd builds.\n" +
			"The version you are on is rebuilt now; other versions rebuild the next time they are used.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgs, err := podman.ParseApkDeps(strings.Join(args, " "))
			if err != nil {
				return err
			}
			if err := rejectPerVersionFlag(cmd); err != nil {
				return err
			}
			version, err := phpPkgVersion("")
			if err != nil {
				return err
			}

			// Only what this command actually added is reverted on failure, so a
			// bad name in the list cannot rip out a package that was already
			// declared and working.
			var added []string
			if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
				for _, p := range pkgs {
					if !slices.Contains(c.GetPackages(), p) {
						c.AddPackage(p)
						added = append(added, p)
					}
				}
			}); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			feedback.Begin()
			feedback.Line("adding packages to every PHP version: " + feedback.Val(strings.Join(pkgs, " ")))
			// Re-reads rather than saving the pre-build copy: the rebuild below
			// records what each version realised while this command waits.
			revert := func() {
				if len(added) == 0 {
					return
				}
				if saveErr := config.UpdateGlobal(func(c *config.GlobalConfig) {
					for _, p := range added {
						c.RemovePackage(p)
					}
				}); saveErr != nil {
					feedback.Warn("reverting config: %v", saveErr)
				}
			}
			if err := podman.RebuildFPMImage(version, false); err != nil {
				revert()
				return fmt.Errorf("rebuild failed (config reverted): %w", err)
			}
			// The build installs packages tolerantly so the legacy images survive
			// a name they do not have, so a typo would otherwise report success.
			missing, err := podman.VerifyPackagesInstalled(version, pkgs)
			if err != nil {
				return err
			}
			if len(missing) > 0 {
				revert()
				return fmt.Errorf("packages not installed on PHP %s (config reverted): %s\ncheck the names exist in Alpine's repositories", version, strings.Join(missing, ", "))
			}

			applyPHPImageChange(version)
			feedback.Done("packages installed for PHP " + version)
			reportOtherVersionsStale(version)
			return nil
		},
	}
	cmd.Flags().String("php", "", "deprecated: packages now apply to every PHP version")
	_ = cmd.Flags().MarkDeprecated("php", "packages now apply to every PHP version")
	return cmd
}

func newPhpPkgRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <package...>",
		Short: "Remove extra Alpine packages from every PHP-FPM image",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectPerVersionFlag(cmd); err != nil {
				return err
			}
			version, err := phpPkgVersion("")
			if err != nil {
				return err
			}
			if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
				for _, p := range args {
					c.RemovePackage(p)
				}
			}); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			feedback.Begin()
			feedback.Line("removing packages from every PHP version: " + feedback.Val(strings.Join(args, " ")))
			if err := podman.RebuildFPMImage(version, false); err != nil {
				return err
			}
			applyPHPImageChange(version)
			feedback.Done("packages removed for PHP " + version)
			reportOtherVersionsStale(version)
			return nil
		},
	}
	cmd.Flags().String("php", "", "deprecated: packages now apply to every PHP version")
	_ = cmd.Flags().MarkDeprecated("php", "packages now apply to every PHP version")
	return cmd
}

func newPhpPkgListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your extra Alpine packages and where they did not install",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			pkgs := cfg.GetPackages()
			if len(pkgs) == 0 {
				fmt.Println("No extra packages configured.")
				return nil
			}
			fmt.Println("Declared, for every PHP version:")
			for _, p := range pkgs {
				fmt.Printf("  - %s\n", p)
			}
			printPerVersionStatus(cfg, packagesOf)
			return nil
		},
	}
	return cmd
}

// rejectPerVersionFlag turns the old --php form into a teachable error, for the
// same reason php:ext rejects its positional version.
func rejectPerVersionFlag(cmd *cobra.Command) error {
	v, _ := cmd.Flags().GetString("php")
	if v == "" {
		return nil
	}
	return fmt.Errorf("packages now apply to every PHP version, so --php is gone.\n"+
		"Drop the flag, then run 'lerd php:rebuild %s' if you want that image rebuilt right away", v)
}

// restartFPMUnit restarts the FPM container for a PHP version after an image
// rebuild, printing a manual hint if the restart fails.
func restartFPMUnit(version string) {
	unit := "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
	if err := podman.RestartUnit(unit); err != nil {
		feedback.Warn("restart %s: %v", unit, err)
		fmt.Printf("Run: systemctl --user restart %s\n", unit)
	} else {
		feedback.Note("restarted " + unit)
	}
}
