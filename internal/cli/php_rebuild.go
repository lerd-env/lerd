package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/lifecycle"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
	"github.com/spf13/cobra"
)

// frankenRestart pairs a FrankenPHP container unit with its (normalized) image
// version so the restart can confirm that version's image actually built.
type frankenRestart struct {
	unit    string
	version string
}

// frankenPHPRebuildTargets returns the distinct normalized PHP versions to
// rebuild, and the container units (with their versions) to restart, for
// non-paused FrankenPHP sites whose version is among the requested ones, so
// php:rebuild rebuilds and restarts only what's affected.
func frankenPHPRebuildTargets(requested []string) (versions []string, units []frankenRestart) {
	want := map[string]bool{}
	for _, v := range requested {
		want[config.NormalizeFrankenPHPVersion(v)] = true
	}
	reg, err := config.LoadSites()
	if err != nil {
		return nil, nil
	}
	seenVer := map[string]bool{}
	for _, s := range reg.Sites {
		if s.Ignored || s.Paused || !s.IsFrankenPHP() {
			continue
		}
		v := config.NormalizeFrankenPHPVersion(s.PHPVersion)
		if !want[v] {
			continue
		}
		if !seenVer[v] {
			seenVer[v] = true
			versions = append(versions, v)
		}
		units = append(units, frankenRestart{unit: podman.FrankenPHPContainerName(s.Name), version: v})
	}
	return versions, units
}

// rebuildFrankenPHPForVersion rebuilds and restarts the derived FrankenPHP image
// for any non-paused FrankenPHP site on this version. php:ext/php:pkg changes
// rebuild the FPM image directly; without this, the same change would silently
// never reach Octane (FrankenPHP) sites until an explicit php:rebuild. The hash
// now tracks the custom exts/packages, so a force-less rebuild detects the drift
// and is a no-op when nothing actually changed. A FrankenPHP build failure is
// only warned about: the change is already live under FPM and config stays
// committed, so reverting it here would undo the successful FPM install too.
func rebuildFrankenPHPForVersion(version string) {
	versions, units := frankenPHPRebuildTargets([]string{version})
	if len(versions) == 0 {
		return
	}
	for _, v := range versions {
		feedback.Note("rebuilding FrankenPHP " + v + " image for Octane sites")
		if err := podman.BuildFrankenPHPImage(v, false, os.Stdout); err != nil {
			feedback.Warn("rebuild FrankenPHP %s image: %v", v, err)
			fmt.Printf("       the change is live under FPM; run 'lerd php:rebuild %s' to retry the Octane image\n", v)
			return
		}
	}
	restartFrankenPHPUnits(units)
}

// applyPHPImageChange propagates a freshly-rebuilt FPM image for a version to
// every runtime: it restarts the shared FPM container and rebuilds/restarts any
// FrankenPHP (Octane) site on that version. All php:ext/php:pkg handlers funnel
// through here after their FPM rebuild, so none can forget the FrankenPHP step
// and silently leave Octane sites on the old extension/package set.
func applyPHPImageChange(version string) {
	restartFPMUnit(version)
	rebuildFrankenPHPForVersion(version)
}

// restartFrankenPHPUnits restarts each FrankenPHP container onto its freshly
// built image, skipping any whose image isn't present so a failed build doesn't
// bounce a running container onto a missing image.
func restartFrankenPHPUnits(units []frankenRestart) {
	for _, u := range units {
		if podman.RunSilent("image", "exists", podman.FrankenPHPImageName(u.version)) != nil {
			feedback.Warn("%s: image not built, leaving container as-is", u.unit)
			continue
		}
		if err := podman.RestartUnit(u.unit); err != nil {
			feedback.Warn("restart %s: %v", u.unit, err)
		} else {
			feedback.Note("restarted " + u.unit)
		}
	}
}

// RebuildPHPVersion force-rebuilds one version's image against the current
// prebuilt base and brings everything running on it back up, streaming the
// build to w. The dashboard's rebuild action goes through here so it means the
// same thing as `lerd php:rebuild <version>`.
func RebuildPHPVersion(version string, w io.Writer) error {
	version, err := config.NormalizePHPVersion(version)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Rebuilding PHP %s image...\n", version)
	if err := podman.RebuildFPMImageTo(version, false, w); err != nil {
		return err
	}
	if err := podman.StoreFPMHash(); err != nil {
		fmt.Fprintf(w, "  WARN: storing PHP-FPM image hash: %v\n", err)
	}
	fmt.Fprintln(w, "Restarting containers...")
	running := runningInContainerWorkers()
	applyPHPImageChange(version)
	restartInContainerWorkers(running)
	fmt.Fprintf(w, "PHP %s image rebuilt.\n", version)
	return nil
}

// runningInContainerWorkers snapshots which of the declared podman-exec worker
// units are running. Callers take it before restarting an FPM container: BindsTo
// stops those workers along with the container, so afterwards there is no way
// left to tell which ones the user actually had up.
func runningInContainerWorkers() []string {
	var out []string
	for _, unit := range append(append(lifecycle.RegisteredReverbUnits(), lifecycle.RegisteredQueueUnits()...), lifecycle.RegisteredScheduleUnits()...) {
		if lerdSystemd.IsServiceActive(unit) {
			out = append(out, unit)
		}
	}
	return out
}

// restartInContainerWorkers brings back the workers a snapshot found running.
// BindsTo stops them when the FPM container stops but does not bring them back
// when it returns, so a rebuild has to do it explicitly.
func restartInContainerWorkers(units []string) {
	for _, unit := range units {
		if err := lerdSystemd.RestartService(unit); err != nil {
			feedback.Warn("restart %s: %v", unit, err)
		} else {
			feedback.Note("restarted " + unit)
		}
	}
}

// registerPHPVersionForRebuild writes the FPM quadlet for a version this machine
// has never installed, so an explicit rebuild of it registers the version rather
// than building an image nothing points at. Every surface that reports a missing
// version sends the user here, and without the unit the build was invisible:
// php:list omitted the version, the shims still called it uninstalled, and the
// restart at the end of the rebuild failed on a unit that did not exist. Writing
// it first mirrors the ensure path, which registers before it builds so a failed
// build still leaves the version known.
func registerPHPVersionForRebuild(version string) error {
	if phpPkg.IsInstalled(version) {
		return nil
	}
	if err := writeFPMQuadlet(version); err != nil {
		return fmt.Errorf("registering PHP %s: %w", version, err)
	}
	feedback.Note("registered PHP " + version + ", which was not installed")
	return nil
}

// NewPhpRebuildCmd returns the php:rebuild command.
func NewPhpRebuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "php:rebuild [version]",
		Short: "Force-rebuild PHP-FPM image(s)",
		Long:  "Force-rebuilds lerd PHP-FPM container images. Pulls a pre-built base from ghcr.io by default; pass --local to build entirely from source.\nPass a version (e.g. 8.3) to rebuild only that version, or omit to rebuild all installed versions.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runPhpRebuild,
	}
	cmd.Flags().Bool("local", false, "Build images locally instead of pulling pre-built base images")
	return cmd
}

func runPhpRebuild(cmd *cobra.Command, args []string) error {
	local, _ := cmd.Flags().GetBool("local")
	var versions []string

	if len(args) == 1 {
		v, err := config.NormalizePHPVersion(args[0])
		if err != nil {
			return err
		}
		if err := registerPHPVersionForRebuild(v); err != nil {
			return err
		}
		versions = []string{v}
	} else {
		var err error
		versions, err = phpPkg.ListInstalled()
		if err != nil {
			return fmt.Errorf("listing PHP versions: %w", err)
		}
	}

	if len(versions) == 0 {
		feedback.Line("no PHP versions installed")
		return nil
	}

	feedback.Begin()
	jobs := make([]BuildJob, 0, len(versions))
	for _, v := range versions {
		ver := v
		jobs = append(jobs, BuildJob{
			Label: "PHP " + ver,
			Run:   func(w io.Writer) error { return podman.RebuildFPMImageTo(ver, local, w) },
		})
	}
	// Rebuild the derived FrankenPHP image for any requested version a FrankenPHP
	// site uses, so its baked extensions track the FPM set, then restart those
	// containers onto the new image.
	fpVersions, fpUnits := frankenPHPRebuildTargets(versions)
	for _, v := range fpVersions {
		ver := v
		jobs = append(jobs, BuildJob{
			Label: "FrankenPHP " + ver,
			Run:   func(w io.Writer) error { return podman.BuildFrankenPHPImage(ver, true, w) },
		})
	}
	RunParallel(jobs) //nolint:errcheck — individual failures printed by RunParallel

	restartFrankenPHPUnits(fpUnits)

	// Store the new Containerfile hash so future updates know images are current.
	if err := podman.StoreFPMHash(); err != nil {
		feedback.Warn("could not store image hash: %v", err)
	}

	label := "PHP-FPM images"
	if len(versions) == 1 {
		label = "PHP " + versions[0] + " image"
	}
	feedback.Line("restarting containers")
	running := runningInContainerWorkers()
	for _, v := range versions {
		unit := "lerd-php" + strings.ReplaceAll(v, ".", "") + "-fpm"
		if err := podman.RestartUnit(unit); err != nil {
			feedback.Warn("restart %s: %v", unit, err)
		} else {
			feedback.Note("restarted " + unit)
		}
	}

	restartInContainerWorkers(running)

	feedback.Done(label + " rebuilt")
	return nil
}
