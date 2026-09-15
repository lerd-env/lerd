package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// SupportedPHPVersions re-exports the canonical list from internal/config, the
// single source of truth shared with the FrankenPHP image picker.
var SupportedPHPVersions = config.SupportedPHPVersions

// LegacyPHPVersions is the frozen legacy tier built from Alpine 3.16 with an old
// bundled Node. Kept here next to SupportedPHPVersions so the single definition
// of "legacy" is reused (e.g. pest:browser, which needs a modern Node) instead
// of being duplicated as literals elsewhere.
var LegacyPHPVersions = []string{"7.4", "8.0"}

// IsSupportedPHPVersion reports whether v is a version lerd can install.
func IsSupportedPHPVersion(v string) bool {
	return config.IsSupportedPHPVersion(v)
}

// IsLegacyPHPVersion reports whether v belongs to the frozen legacy tier.
func IsLegacyPHPVersion(v string) bool {
	for _, s := range LegacyPHPVersions {
		if s == v {
			return true
		}
	}
	return false
}

// InstallPHPVersion builds the FPM image for the given version, registers its
// quadlet and starts the service, streaming build output to w. It is the
// programmatic entry point behind the UI's "add PHP version" flow.
func InstallPHPVersion(version string, w io.Writer) error {
	version, err := config.NormalizePHPVersion(version)
	if err != nil {
		return err
	}
	// Always emit a line so a streamed install shows progress even when the
	// image is already built and the build step produces no output.
	fmt.Fprintf(w, "Installing PHP %s...\n", version)
	if err := ensureFPMQuadletTo(version, w); err != nil {
		return err
	}
	fmt.Fprintf(w, "PHP %s installed and started.\n", version)
	return nil
}

// NewFetchCmd returns the fetch command.
func NewFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch [version...]",
		Short: "Pre-build PHP FPM images so first use isn't slow",
		Long:  "Pulls pre-built PHP-FPM base images from ghcr.io and applies local layers (mkcert CA, custom extensions).\nPass --local to skip the pull and build entirely from source.\nSkips any version whose image already exists.\nWith no arguments it builds the released versions; a prerelease only builds when named.",
		RunE:  runFetch,
	}
	cmd.Flags().Bool("local", false, "Build images locally instead of pulling pre-built base images")
	return cmd
}

func runFetch(cmd *cobra.Command, args []string) error {
	local, _ := cmd.Flags().GetBool("local")

	versions := make([]string, 0, len(args))
	for _, a := range args {
		v, err := config.NormalizePHPVersion(a)
		if err != nil {
			return err
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		versions = config.StablePHPVersions()
	}

	var rebuiltMu sync.Mutex
	var rebuilt []string
	jobs := make([]BuildJob, len(versions))
	for i, v := range versions {
		ver := v
		jobs[i] = BuildJob{
			Label: "PHP " + ver,
			Run: func(w io.Writer) error {
				changed, err := podman.BuildFPMImageTo(ver, local, w)
				if changed {
					rebuiltMu.Lock()
					rebuilt = append(rebuilt, ver)
					rebuiltMu.Unlock()
				}
				return err
			},
		}
	}

	// Only versions whose image is stale actually build, so only those are
	// disclosed as a download.
	var pending []string
	for _, v := range versions {
		if !podman.FPMImageCurrent(v) {
			pending = append(pending, v)
		}
	}
	phpBuildPlan(pending, local, "requested by lerd fetch").Fill().Report(os.Stdout)

	if err := RunParallel(jobs); err != nil {
		feedback.Warn("some images failed to build: %v", err)
	}
	restartRebuiltFPMUnits(rebuilt)
	feedback.Done("all requested PHP images ready")
	// An image is not yet a runtime: php:list and `lerd new` read the quadlet,
	// so a fetched version they still call missing has to be named here rather
	// than contradicted by the next command.
	if installed, err := phpPkg.ListInstalled(); err == nil {
		if missing := phpVersionsWithoutRuntime(versions, installed); len(missing) > 0 {
			if len(missing) == 1 {
				feedback.Note(fmt.Sprintf("PHP %s has an image but no runtime yet — run 'lerd php:rebuild %s' to install it", missing[0], missing[0]))
			} else {
				feedback.Note(fmt.Sprintf("PHP %s have images but no runtime yet — run 'lerd php:rebuild <version>' to install one", strings.Join(missing, ", ")))
			}
		}
	}
	return nil
}

// phpVersionsWithoutRuntime returns the requested versions that have no
// installed runtime behind them, in the order they were requested.
func phpVersionsWithoutRuntime(requested, installed []string) []string {
	have := make(map[string]bool, len(installed))
	for _, v := range installed {
		have[v] = true
	}
	var missing []string
	for _, v := range requested {
		if !have[v] {
			missing = append(missing, v)
		}
	}
	return missing
}

// restartRebuiltFPMUnits bounces the containers of versions whose image this run
// replaced, so a fetch that quietly rebuilt a stale image doesn't leave the
// running container on the image it superseded. Versions that aren't up are left
// alone: fetch pre-builds images and is not a reason to start anything.
func restartRebuiltFPMUnits(versions []string) {
	for _, v := range versions {
		unit := "lerd-php" + strings.ReplaceAll(v, ".", "") + "-fpm"
		if running, _ := fpmContainerRunning(unit); !running {
			continue
		}
		if err := restartUnitFn(unit); err != nil {
			feedback.Warn("restart %s: %v", unit, err)
		} else {
			feedback.Note("restarted " + unit)
		}
	}
}
