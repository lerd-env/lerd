package siteops

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// Indirection points so tests can drive the funnel's decisions without the
// framework store or a real FrankenPHP build.
var (
	finishFrankenPHPFn = FinishFrankenPHPLink
	imageGapFn         = imageGap
	imageStaleFn       = podman.FPMImageStale
	imageExistsFn      = podman.FPMImageExists

	frameworkPHPRange = func(site *config.Site) (string, string) {
		if site.Framework == "" {
			return "", ""
		}
		fw, ok := config.GetFrameworkForDir(site.Framework, site.Path)
		if !ok {
			return "", ""
		}
		return fw.PHP.Min, fw.PHP.Max
	}
)

// PHPVersionOpts varies what SetSitePHPVersion targets.
type PHPVersionOpts struct {
	// Branch targets a worktree of the site rather than the site itself.
	Branch string
}

// PHPVersionResult reports what the switch actually did, so each caller can
// render it in its own idiom rather than the funnel printing for everyone.
type PHPVersionResult struct {
	Requested string // what the caller asked for
	Version   string // what was applied, after clamping
	Clamped   bool   // the framework's range overrode the request
	Demoted   bool   // the site fell back from FrankenPHP to FPM

	// Missing lists declared entries this version's image tried to load and
	// could not: mongodb below 8.1, or anything the Alpine 3.16 legacy images
	// cannot install. A rebuild will not fix these.
	Missing []string
	// Stale reports that the image was built from an older declared set, so it
	// predates entries the user has since added. A rebuild does fix this.
	Stale bool
	// NotInstalled reports that lerd has never built this version at all.
	NotInstalled bool
}

// SetSitePHPVersion switches a site (or one of its worktrees) to a PHP version
// and runs every step that switch depends on. It is the single source of truth
// for "what happens when a site changes PHP version"; CLI, UI, and MCP all call
// it, so a step added here applies everywhere.
//
// Steps:
//  1. Refuse runtimes that have no PHP version of their own.
//  2. Clamp to the framework's supported range, so the pin never advertises a
//     version the watcher clamps back on its next pass.
//  3. Pin .php-version and .lerd.yaml.
//  4. Persist site.PHPVersion to the registry.
//  5. Re-link FrankenPHP, or fall back to FPM below its minimum version.
//  6. Ensure the FPM quadlet and xdebug ini exist for the new version.
//  7. Regenerate the nginx vhost (SSL or plain) and reload.
//
// It never builds an image and never prompts: callers own both, because only
// they know whether a human is waiting.
func SetSitePHPVersion(site *config.Site, version string, opts PHPVersionOpts) (PHPVersionResult, error) {
	res := PHPVersionResult{Requested: version, Version: version}

	if !config.IsSupportedPHPVersion(version) {
		return res, fmt.Errorf("unsupported PHP version %q (supported: %s)", version, strings.Join(config.SupportedPHPVersions, ", "))
	}
	if site.IsCustomContainer() {
		return res, fmt.Errorf("site %q runs a custom container, which defines its own PHP runtime", site.Name)
	}
	if site.IsHostProxy() {
		return res, fmt.Errorf("site %q is a host-proxy site, which runs your dev command on the host", site.Name)
	}

	min, max := frameworkPHPRange(site)
	if clamped := php.ClampToRange(version, min, max); clamped != version {
		res.Version, res.Clamped = clamped, true
		version = clamped
	}
	gap := imageGapFn(version)
	res.Missing, res.Stale, res.NotInstalled = gap.missing, gap.stale, gap.notInstalled

	if opts.Branch != "" {
		return res, setWorktreePHPVersion(site, opts.Branch, version)
	}

	if err := PinPHPVersionFile(site.Path, version); err != nil {
		return res, fmt.Errorf("writing .php-version: %w", err)
	}
	_ = config.SetProjectPHPVersion(site.Path, version)

	site.PHPVersion = version
	if err := config.AddSite(*site); err != nil {
		return res, fmt.Errorf("updating site registry: %w", err)
	}

	if site.IsFrankenPHP() {
		// FrankenPHP publishes no image below 8.2; building one normalizes the
		// version up and silently runs a different PHP than the site reports.
		if !config.IsFrankenPHPVersion(version) {
			if err := DemoteFrankenPHPToFPM(site); err != nil {
				return res, err
			}
			res.Demoted = true
			return res, nil
		}
		if err := finishFrankenPHPFn(*site); err != nil {
			return res, fmt.Errorf("re-linking FrankenPHP site: %w", err)
		}
		return res, nil
	}

	if err := podman.WriteFPMQuadlet(version); err == nil {
		_ = podman.DaemonReloadFn()
	}
	_ = podman.EnsureXdebugIni(version) // non-fatal if the version isn't built yet

	if err := regenerateSiteVhost(site, version); err != nil {
		return res, err
	}
	_ = podman.RewriteFPMQuadlets()
	_ = podman.WriteContainerHosts()
	if err := nginxReloadFn(); err != nil {
		return res, fmt.Errorf("reloading nginx: %w", err)
	}

	// Changing version starts no systemd unit, so the shared hook would not
	// otherwise fire and every open dashboard would keep showing the old
	// version against a vhost already serving the new one. FinishLink notifies
	// for the same reason on the link path.
	if podman.AfterUnitChange != nil {
		podman.AfterUnitChange("site:" + site.Name)
	}
	return res, nil
}

// imageGap reports how a version's image falls short of the declared set. The
// three answers are deliberately distinct, because the user's next move differs
// for each: no image at all, an image predating the set (rebuild fixes it), and
// an image that genuinely could not build part of it (rebuild will not).
func imageGap(version string) (gap imageGapResult) {
	// Whether an image exists has nothing to do with the declared set, so this
	// is checked first: most users declare nothing, and moving one of their
	// sites onto an unbuilt version is exactly when they need to be told. The
	// image is the question, not php.ListInstalled: that reads quadlet files,
	// and this funnel writes one itself, so a version would look installed from
	// the moment it was first switched to and never warn again.
	if !imageExistsFn(version) {
		gap.notInstalled = true
		return gap
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return gap
	}
	declared := append(slices.Clone(cfg.GetExtensions()), cfg.GetPackages()...)
	if len(declared) == 0 {
		return gap
	}
	if imageStaleFn(version) {
		gap.stale = true
		return gap
	}
	gap.missing = cfg.MissingFromImage(version, declared)
	return gap
}

// imageGapResult is imageGap's answer, kept as one value so the seam tests swap
// stays readable.
type imageGapResult struct {
	missing      []string
	stale        bool
	notInstalled bool
}

// regenerateSiteVhost rewrites the site's vhost with the given PHP version,
// picking the secured or plain template from the site's TLS state.
func regenerateSiteVhost(site *config.Site, version string) error {
	if site.Secured {
		if err := secureCertFn(*site); err != nil {
			return fmt.Errorf("regenerating SSL vhost: %w", err)
		}
		return nil
	}
	if err := nginx.GenerateVhost(*site, version); err != nil {
		return fmt.Errorf("regenerating vhost: %w", err)
	}
	return nil
}

// setWorktreePHPVersion pins the override on a single worktree and regenerates
// just that worktree's vhost, so the next request lands on the new FPM upstream.
// The parent site's own version is untouched.
func setWorktreePHPVersion(site *config.Site, branch, version string) error {
	worktrees, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return fmt.Errorf("detecting worktrees: %w", err)
	}
	for _, wt := range worktrees {
		if wt.Branch != branch {
			continue
		}
		if err := PinPHPVersionFile(wt.Path, version); err != nil {
			return fmt.Errorf("writing .php-version: %w", err)
		}
		if err := config.SetWorktreePHPVersion(wt.Path, version); err != nil {
			return fmt.Errorf("updating .lerd.yaml: %w", err)
		}
		if site.Secured {
			err = nginx.GenerateWorktreeSSLVhost(wt.Domain, wt.Path, version, site.PrimaryDomain(), site.Name, wt.Branch)
		} else {
			err = nginx.GenerateWorktreeVhost(wt.Domain, wt.Path, version, site.Name, wt.Branch)
		}
		if err != nil {
			return fmt.Errorf("regenerating worktree vhost: %w", err)
		}
		if err := nginxReloadFn(); err != nil {
			return fmt.Errorf("reloading nginx: %w", err)
		}
		return nil
	}
	return fmt.Errorf("worktree %q not found", branch)
}

// PinPHPVersionFile writes the version lerd resolved into the project's
// .php-version, so the file, the site registry and the FPM container never
// disagree. A file that already matches is left alone: rewriting it would wake
// the watcher and trigger a pointless queue:restart on every link.
func PinPHPVersionFile(dir, version string) error {
	if version == "" {
		return nil
	}
	path := filepath.Join(dir, ".php-version")
	if current, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(current)) == version {
		return nil
	}
	return os.WriteFile(path, []byte(version+"\n"), 0644)
}
