package siteops

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// fpmReadyTimeout bounds the wait for a freshly started FPM container. Long
// enough for a cold container on a slow disk, short enough that a broken image
// cannot hold the command open.
const fpmReadyTimeout = 30 * time.Second

// Indirection points so tests can drive the funnel's decisions without the
// framework store or a real FrankenPHP build.
var (
	finishFrankenPHPFn = FinishFrankenPHPLink
	imageGapFn         = imageGap
	ensureFPMReadyFn   = podman.EnsureFPMReady
	imageStaleFn       = podman.FPMImageStale
	imageExistsFn      = podman.FPMImageExists
	detectWorktreesFn  = gitpkg.DetectWorktrees

	// phpConstraintsFor is everything the site has to satisfy at once. The
	// framework definition's range is one claim, the project's own composer
	// requirement is the other, and both hold. A borrowed definition describes a
	// different release, so it is left out, and so is a real one whose range
	// cannot be met alongside what the project requires: the
	// definition describes the framework, composer describes the app that has to
	// boot, and a version the app rejects fails at the first request. The two
	// are compared as constraints, not against what is installed, so the answer
	// does not change with the machine.
	// getFrameworkFn is the definition lookup, a seam so the constraint choice
	// can be tested without a framework store on disk.
	getFrameworkFn = config.GetFrameworkForDir

	phpConstraintsFor = func(site *config.Site) []string {
		project := php.ComposerPHPConstraint(site.Path)
		framework := ""
		if site.Framework != "" {
			if fw, ok := getFrameworkFn(site.Framework, site.Path); ok && !fw.VersionGuessed {
				framework = phpRangeConstraint(fw.PHP.Min, fw.PHP.Max)
			}
		}
		switch {
		case framework == "":
			return compactConstraints(project)
		case project == "":
			return compactConstraints(framework)
		case !php.ConstraintsOverlap(framework, project):
			return compactConstraints(project)
		}
		return compactConstraints(framework, project)
	}
)

// compactConstraints drops the empty entries, so a caller can hand over whatever
// it found without checking each one.
func compactConstraints(constraints ...string) []string {
	out := make([]string, 0, len(constraints))
	for _, c := range constraints {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// phpRangeConstraint renders a framework definition's min/max as the constraint
// shape everything else in this path speaks.
func phpRangeConstraint(min, max string) string {
	switch {
	case min != "" && max != "":
		return ">=" + min + " <=" + max
	case min != "":
		return ">=" + min
	case max != "":
		return "<=" + max
	}
	return ""
}

// PHPVersionOpts varies what SetSitePHPVersion targets.
type PHPVersionOpts struct {
	// Branch targets a worktree of the site rather than the site itself.
	Branch string
	// Force applies a version the site's constraints refuse. The caller has
	// told the user what they are overriding.
	Force bool
}

// PHPVersionResult reports what the switch actually did, so each caller can
// render it in its own idiom rather than the funnel printing for everyone.
type PHPVersionResult struct {
	Requested string // what the caller asked for
	Version   string // what was applied
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

// PHPRangeError reports a version the site is not allowed to run, and what it
// would have to satisfy to be allowed. It carries the facts rather than a
// sentence, so each caller phrases the refusal in its own idiom.
type PHPRangeError struct {
	Site        string
	Requested   string
	Constraints []string
	// Best is the newest installed version that satisfies every constraint, or
	// "" when this machine has none.
	Best string
}

func (e *PHPRangeError) Error() string {
	msg := fmt.Sprintf("PHP %s is outside what %s can run (needs %s)",
		e.Requested, e.Site, strings.Join(e.Constraints, " and "))
	if e.Best != "" {
		msg += fmt.Sprintf("; the closest installed version is %s", e.Best)
	}
	return msg
}

// SetSitePHPVersion switches a site (or one of its worktrees) to a PHP version
// and runs every step that switch depends on. It is the single source of truth
// for "what happens when a site changes PHP version"; CLI, UI, and MCP all call
// it, so a step added here applies everywhere.
//
// Steps:
//  1. Refuse runtimes that have no PHP version of their own.
//  2. Refuse a version the framework definition or the project's own composer
//     requirement rules out, unless the caller forces it. Nothing is written on
//     a refusal, so a request lerd cannot honour never reaches the files the
//     project commits.
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

	norm, err := config.NormalizePHPVersion(version)
	if err != nil {
		return res, err
	}
	version = norm
	res.Version = version
	if site.IsCustomContainer() {
		return res, fmt.Errorf("site %q runs a custom container, which defines its own PHP runtime", site.Name)
	}
	if site.IsHostProxy() {
		return res, fmt.Errorf("site %q is a host-proxy site, which runs your dev command on the host", site.Name)
	}

	if constraints := phpConstraintsFor(site); !opts.Force && !php.SatisfiesAll(version, constraints...) {
		return res, &PHPRangeError{
			Site:        site.Name,
			Requested:   version,
			Constraints: constraints,
			Best:        php.BestInstalledFor(constraints...),
		}
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

	// The vhost now points at the new version's backend, so the site 502s until
	// that container is accepting connections. Waiting here means the first
	// request after a switch is served, which matters most the first time a
	// freshly built version is used and nothing has started it yet.
	_ = ensureFPMReadyFn(res.Version, fpmReadyTimeout)

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
	worktrees, err := detectWorktreesFn(site.Path, site.PrimaryDomain())
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
		// Same runtime setup the site path does: the vhost below points at the
		// version's FPM container, which need not exist on this machine yet.
		if err := podman.WriteFPMQuadlet(version); err == nil {
			_ = podman.DaemonReloadFn()
		}
		_ = podman.EnsureXdebugIni(version)
		if site.Secured {
			err = nginx.GenerateWorktreeSSLVhost(wt.Domain, wt.Path, version, site.PrimaryDomain(), site.Name, wt.Branch)
		} else {
			err = nginx.GenerateWorktreeVhost(wt.Domain, wt.Path, version, site.Name, wt.Branch)
		}
		if err != nil {
			return fmt.Errorf("regenerating worktree vhost: %w", err)
		}
		_ = podman.RewriteFPMQuadlets()
		_ = podman.WriteContainerHosts()
		if err := nginxReloadFn(); err != nil {
			return fmt.Errorf("reloading nginx: %w", err)
		}
		if podman.AfterUnitChange != nil {
			podman.AfterUnitChange("site:" + site.Name)
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
