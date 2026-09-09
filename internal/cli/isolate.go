package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewIsolateCmd returns the isolate command.
func NewIsolateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "isolate <version>",
		Short: "Pin the PHP version for the current directory",
		Args:  cobra.ExactArgs(1),
		RunE:  runIsolate,
	}
	cmd.Flags().BoolVar(&isolateForce, "force", false, "pin a version the framework or the project's composer.json rules out")
	return cmd
}

var isolateForce bool

// pinRefusal turns a refused pin into the command's own answer, naming the flag
// that overrides it. Nothing was written, so the user is being told what to do
// next rather than what was done to their project.
func pinRefusal(err error, requested string) error {
	var rangeErr *siteops.PHPRangeError
	if !errors.As(err, &rangeErr) {
		return err
	}
	return fmt.Errorf("%w\n       run 'lerd isolate %s --force' to pin it anyway", err, requested)
}

func runIsolate(_ *cobra.Command, args []string) error {
	version, err := config.NormalizePHPVersion(args[0])
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Worktree path: the override travels with the branch, so the parent site's
	// own version is left alone.
	if site, branch, ok := FindParentSiteForWorktree(cwd); ok {
		res, err := siteops.SetSitePHPVersion(site, version, siteops.PHPVersionOpts{Branch: branch, Force: isolateForce})
		if err != nil {
			return pinRefusal(err, args[0])
		}
		feedback.Begin()
		feedback.Done("PHP pinned to " + feedback.Val(res.Version) + " · worktree " + branch + " of " + site.Name)
		reportImageGap(res)
		return nil
	}

	// An unlinked directory has no site to switch, so the pin is all there is
	// to write. link picks it up when the directory is eventually linked.
	site, err := config.FindSiteByPath(cwd)
	if err != nil {
		if err := siteops.PinPHPVersionFile(cwd, version); err != nil {
			return fmt.Errorf("writing .php-version: %w", err)
		}
		_ = config.SetProjectPHPVersion(cwd, version)
		feedback.Begin()
		feedback.Done("PHP pinned to " + feedback.Val(version))
		if note := phpVersionNotInstalledNote(version); note != "" {
			feedback.Note(note)
		}
		return nil
	}

	res, err := siteops.SetSitePHPVersion(site, version, siteops.PHPVersionOpts{Force: isolateForce})
	if err != nil {
		return pinRefusal(err, args[0])
	}
	feedback.Begin()
	feedback.Done("PHP pinned to " + feedback.Val(res.Version))
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

// phpVersionNotInstalledNote describes a pin that names a PHP version this
// machine does not have, and returns "" when it does. A pin is legitimate on its
// own — link provisions the version when the directory becomes a site — but
// saying only "pinned" left every command run there next failing on a version
// the user was never told was missing.
func phpVersionNotInstalledNote(version string) string {
	if phpDet.IsInstalled(version) {
		return ""
	}
	return "PHP " + version + " has no image yet; 'lerd link' provisions it here, or run 'lerd php:rebuild " + version + "' to add it now"
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
