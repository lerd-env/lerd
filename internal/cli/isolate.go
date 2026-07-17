package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewIsolateCmd returns the isolate command.
func NewIsolateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "isolate <version>",
		Short: "Pin the PHP version for the current directory",
		Args:  cobra.ExactArgs(1),
		RunE:  runIsolate,
	}
}

func runIsolate(_ *cobra.Command, args []string) error {
	version := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Worktree path: the override travels with the branch, so the parent site's
	// own version is left alone.
	if site, branch, ok := FindParentSiteForWorktree(cwd); ok {
		res, err := siteops.SetSitePHPVersion(site, version, siteops.PHPVersionOpts{Branch: branch})
		if err != nil {
			return err
		}
		feedback.Begin()
		feedback.Done("PHP pinned to " + feedback.Val(res.Version) + " · worktree " + branch + " of " + site.Name)
		if res.Clamped {
			feedback.Note(res.Requested + " isn't usable here; clamped to " + res.Version)
		}
		reportImageGap(res)
		return nil
	}

	// An unlinked directory has no site to switch, so the pin is all there is
	// to write. link picks it up when the directory is eventually linked.
	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		if !config.IsSupportedPHPVersion(version) {
			return fmt.Errorf("unsupported PHP version %q (supported: %s)", version, strings.Join(config.SupportedPHPVersions, ", "))
		}
		if err := siteops.PinPHPVersionFile(cwd, version); err != nil {
			return fmt.Errorf("writing .php-version: %w", err)
		}
		_ = config.SetProjectPHPVersion(cwd, version)
		feedback.Begin()
		feedback.Done("PHP pinned to " + feedback.Val(version))
		return nil
	}

	res, err := siteops.SetSitePHPVersion(site, version, siteops.PHPVersionOpts{})
	if err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("PHP pinned to " + feedback.Val(res.Version))
	if res.Clamped {
		feedback.Note(res.Requested + " isn't usable here; clamped to " + res.Version + " and updated .lerd.yaml / .php-version")
	}
	if res.Demoted {
		feedback.Note("FrankenPHP has no image for PHP " + res.Version + "; the site now runs on FPM")
	}
	reportImageGap(res)
	// The version the site runs on decides which composer ext-* requirements
	// its image can satisfy, so re-check them here. isolate used to get this
	// from the full re-link it no longer performs.
	if cfg, err := config.LoadGlobal(); err == nil {
		warnMissingExtensions(cwd, site.Name, res.Version, cfg)
	}
	return nil
}

// reportImageGap surfaces what the new version's image is missing. Changing
// version is exactly when a site loses a custom extension, and lerd knows what
// the target image holds, so staying quiet is the bug.
func reportImageGap(res siteops.PHPVersionResult) {
	switch {
	case res.NotInstalled:
		feedback.Note("PHP " + res.Version + " has no image yet; run 'lerd php:rebuild " + res.Version + "' to build it")
	case res.Stale:
		feedback.Warn("PHP %s's image predates your custom extensions and packages", res.Version)
		fmt.Printf("       run 'lerd php:rebuild %s' to bring it up to date\n", res.Version)
	case len(res.Missing) > 0:
		feedback.Warn("PHP %s cannot load: %s", res.Version, strings.Join(res.Missing, ", "))
		fmt.Printf("       they did not build on this version; a rebuild will not change that\n")
	}
}
