package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/spf13/cobra"
)

// NewPhpListCmd returns the php:list command.
func NewPhpListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "php:list",
		Short: "List installed PHP versions",
		RunE:  runPhpList,
	}
}

func runPhpList(_ *cobra.Command, _ []string) error {
	// Under the native runtime the container images are irrelevant: what can
	// actually serve is the binaries on disk, and listing the images would
	// offer versions with nothing behind them.
	if cfg, cerr := config.LoadGlobal(); cerr == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative {
		return printPHPVersions(nativephp.ListInstalled(), cfg.PHP.DefaultVersion)
	}
	versions, err := phpPkg.ListInstalled()
	if err != nil {
		return fmt.Errorf("listing PHP versions: %w", err)
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		fmt.Println("No PHP versions installed in lerd bin directory.")
		fmt.Println("Run 'lerd install' to set up PHP, or 'lerd use <version>' to install a specific version.")
		return nil
	}

	fmt.Println("Installed PHP versions:")
	for _, v := range versions {
		marker := "  "
		if v == cfg.PHP.DefaultVersion {
			marker = "* "
		}
		fmt.Printf("%s%s\n", marker, v)
	}

	return nil
}

// printPHPVersions renders an installed-versions list, marking the default.
func printPHPVersions(versions []string, defaultVersion string) error {
	if len(versions) == 0 {
		fmt.Println("No PHP versions installed.")
		return nil
	}
	fmt.Println("Installed PHP versions:")
	for _, v := range versions {
		marker := "  "
		if v == defaultVersion {
			marker = "* "
		}
		fmt.Printf("%s%s\n", marker, v)
	}
	return nil
}
