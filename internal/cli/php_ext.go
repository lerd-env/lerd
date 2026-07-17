package cli

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

var validExtNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// NewPhpExtCmd returns the php:ext parent command.
func NewPhpExtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "php:ext",
		Short: "Manage custom PHP extensions",
	}
	cmd.AddCommand(newPhpExtAddCmd())
	cmd.AddCommand(newPhpExtRemoveCmd())
	cmd.AddCommand(newPhpExtListCmd())
	return cmd
}

func newPhpExtAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <ext>",
		Short: "Install a custom PHP extension on every PHP version",
		Long: "Adds an extension to your declared set, which applies to every PHP image lerd builds.\n" +
			"The version you are on is rebuilt and verified now; other versions rebuild the next time they are used.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ext := args[0]
			if !validExtNameRe.MatchString(ext) {
				return fmt.Errorf("invalid extension name %q: must contain only letters, digits, hyphens, and underscores", ext)
			}
			if err := rejectPerVersionArg(args[1:], "php:ext add "+ext); err != nil {
				return err
			}
			version, err := phpExtVersion(nil)
			if err != nil {
				return err
			}
			rawDeps, _ := cmd.Flags().GetString("apk-deps")
			deps, err := podman.ParseApkDeps(rawDeps)
			if err != nil {
				return err
			}

			// Re-adding an extension that is already declared must not let a
			// failed verify remove the working one on the way out.
			alreadyDeclared := false
			if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
				alreadyDeclared = slices.Contains(c.GetExtensions(), ext)
				c.AddExtension(ext)
				if len(deps) > 0 {
					c.SetExtApkDeps(ext, deps)
				}
			}); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			feedback.Begin()
			feedback.Line("adding extension " + feedback.Val(ext) + " to every PHP version")
			if len(deps) > 0 {
				feedback.Note("alpine packages: " + strings.Join(deps, " "))
			}
			if err := podman.RebuildFPMImage(version, false); err != nil {
				return err
			}

			// The build records what this version realised, so the revert
			// re-reads rather than saving a copy loaded before the build.
			if err := podman.VerifyExtensionLoaded(version, ext); err != nil {
				if alreadyDeclared {
					return fmt.Errorf("extension %q did not load on PHP %s: %w", ext, version, err)
				}
				if saveErr := config.UpdateGlobal(func(c *config.GlobalConfig) { c.RemoveExtension(ext) }); saveErr != nil {
					feedback.Warn("reverting config: %v", saveErr)
				}
				return fmt.Errorf("extension %q was not installed (config reverted): %w", ext, err)
			}

			applyPHPImageChange(version)

			feedback.Done("extension " + feedback.Val(ext) + " installed for PHP " + version)
			reportOtherVersionsStale(version)
			return nil
		},
	}
	cmd.Flags().String("apk-deps", "", "extra Alpine packages the extension needs to build (space- or comma-separated, e.g. \"libssh2-dev\")")
	return cmd
}

func newPhpExtRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <ext>",
		Short: "Remove a custom PHP extension from every PHP version",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			ext := args[0]
			if !validExtNameRe.MatchString(ext) {
				return fmt.Errorf("invalid extension name %q: must contain only letters, digits, hyphens, and underscores", ext)
			}
			if err := rejectPerVersionArg(args[1:], "php:ext remove "+ext); err != nil {
				return err
			}
			version, err := phpExtVersion(nil)
			if err != nil {
				return err
			}

			if err := config.UpdateGlobal(func(c *config.GlobalConfig) { c.RemoveExtension(ext) }); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			feedback.Begin()
			feedback.Line("removing extension " + feedback.Val(ext) + " from every PHP version")
			if err := podman.RebuildFPMImage(version, false); err != nil {
				return err
			}

			applyPHPImageChange(version)

			feedback.Done("extension " + feedback.Val(ext) + " removed for PHP " + version)
			reportOtherVersionsStale(version)
			return nil
		},
	}
}

func newPhpExtListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your custom PHP extensions and where they did not build",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}

			exts := cfg.GetExtensions()
			if len(exts) == 0 {
				fmt.Println("No custom extensions configured.")
				return nil
			}

			fmt.Println("Declared, for every PHP version:")
			for _, ext := range exts {
				if deps := cfg.GetExtApkDeps(ext); len(deps) > 0 {
					fmt.Printf("  - %s (apk: %s)\n", ext, strings.Join(deps, " "))
				} else {
					fmt.Printf("  - %s\n", ext)
				}
			}
			printPerVersionStatus(cfg, extensionsOf)
			return nil
		},
	}
}

// rejectPerVersionArg turns the old per-version form into a teachable error.
// Silently ignoring it would leave users believing the set is still scoped to
// the version they named, which is the bug this model removes.
func rejectPerVersionArg(rest []string, cmd string) error {
	if len(rest) == 0 {
		return nil
	}
	return fmt.Errorf("extensions and packages now apply to every PHP version, so %q takes no version.\n"+
		"Run '%s', then 'lerd php:rebuild %s' if you want that image rebuilt right away", rest[0], cmd, rest[0])
}

// reportOtherVersionsStale tells the user which installed versions still carry
// the old set. They rebuild on next use; naming them beats a silent wait.
func reportOtherVersionsStale(rebuilt string) {
	installed, err := phpDet.ListInstalled()
	if err != nil {
		return
	}
	var others []string
	for _, v := range installed {
		if v != rebuilt {
			others = append(others, v)
		}
	}
	if len(others) == 0 {
		return
	}
	feedback.Note("PHP " + strings.Join(others, ", ") + " rebuild on next use, or run 'lerd php:rebuild' now")
}

// phpExtVersion resolves the PHP version from args, cwd detection, or global default.
func phpExtVersion(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	v, err := phpDet.DetectVersion(cwd)
	if err != nil {
		cfg, err := config.LoadGlobal()
		if err != nil {
			return "", err
		}
		return cfg.PHP.DefaultVersion, nil
	}
	return v, nil
}
