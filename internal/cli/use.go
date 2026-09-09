package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// nativeInstallFn fetches a version's native build. A seam so the routing can
// be tested without a download.
var nativeInstallFn = func(version string, w io.Writer) error {
	return ensureNativePHPInstalled(&pinnedTools{}, version, w)
}

// NewUseCmd returns the use command.
func NewUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <version>",
		Short: "Set the global PHP version",
		Args:  cobra.ExactArgs(1),
		RunE:  runUse,
	}
}

func runUse(_ *cobra.Command, args []string) error {
	version, err := config.NormalizePHPVersion(args[0])
	if err != nil {
		return err
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	cfg.PHP.DefaultVersion = version
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	feedback.Begin()
	feedback.Done("default PHP set to " + feedback.Val(version))

	// Under the native runtime there is no image to build, so the binary is
	// what has to exist. Without this, `lerd use` set a default version that
	// nothing on the machine could serve.
	if cfg.PHPRuntimeMode() == config.PHPRuntimeNative {
		return nativeInstallFn(version, os.Stdout)
	}

	// Ensure FPM quadlet exists for this version
	if err := ensureFPMQuadlet(version); err != nil {
		feedback.Warn("FPM quadlet for PHP %s: %v", version, err)
	}

	return nil
}
