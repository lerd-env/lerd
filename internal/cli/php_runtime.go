package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/nativephp"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/webserver"
)

// NewPHPRuntimeCmd returns the `lerd php:runtime` command.
func NewPHPRuntimeCmd() *cobra.Command {
	var removeImages bool
	cmd := &cobra.Command{
		Use:   "php:runtime [container|native]",
		Short: "Show or set where PHP runs (macOS only)",
		Long: `Show or set where PHP runs for every site the shared FPM container serves.

  container  PHP-FPM, the CLI and the workers run in containers. Default, and
             the only mode on Linux.

  native     they run on the macOS host instead. Your project is bind-mounted
             into the Podman VM, so a containerised PHP crosses that boundary
             for every file it reads; running on the host removes it.
             Requires PHP 8.1 or newer.

No argument prints the current value. The setting is install-wide: the FPM
container is shared by every site on a PHP version, so sites cannot be moved
one at a time.

Switching rewrites each site's .env for the service addresses the new runtime
can reach, regenerates the vhosts, drops the framework config caches, and
restarts the workers. Going native also stops the shared FPM containers, which
have nothing left to serve, and offers to remove their images. With no
terminal to ask in it names --remove-images instead, which answers yes up
front and also works on an install that is already native.

FrankenPHP sites, custom containers and host-proxy sites are unaffected: they
never used the shared FPM container.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			mode, show, err := phpRuntimeFromArgs(args)
			if err != nil {
				return err
			}
			// With no argument the command only shows the runtime, so the flag
			// would be dropped without a word; refuse it like container does.
			if removeImages && (show || mode != config.PHPRuntimeNative) {
				return errors.New("--remove-images only goes with `lerd php:runtime native`: on the container runtime those images serve every site")
			}
			if show {
				cfg, err := config.LoadGlobal()
				if err != nil {
					return err
				}
				fmt.Printf("PHP runtime: %s\n", cfg.PHPRuntimeMode())
				if !platform.Current.NativePHPRuntime {
					fmt.Println("  (Linux runs PHP in containers; the native runtime is macOS only.)")
				}
				return nil
			}
			if reason := nativeUnavailableFn(); mode == config.PHPRuntimeNative && reason != "" {
				return errors.New(reason)
			}
			prev, err := currentPHPRuntimeFn()
			if err != nil {
				return err
			}
			feedback.Begin()
			if prev == mode {
				feedback.Line("PHP runtime already " + mode)
				if removeImages {
					versions, _ := installedPHPVersionsFn()
					offerFPMImageRemoval(mode, versions, true)
				}
				return nil
			}
			if err := applyPHPRuntimeFn(mode); err != nil {
				return err
			}
			feedback.Done("PHP runtime set to " + feedback.Val(mode) + " (was " + prev + ")")
			// After the switch, never instead of it, the same as the dashboard.
			versions, _ := installedPHPVersionsFn()
			offerFPMImageRemoval(mode, versions, removeImages)
			return nil
		},
	}
	cmd.Flags().BoolVar(&removeImages, "remove-images", false, "with native, remove the PHP-FPM images without asking")
	return cmd
}

// The command's seams, swappable so a test can drive it on any platform
// without switching a real install.
var (
	applyPHPRuntimeFn      = ApplyPHPRuntime
	nativeUnavailableFn    = func() string { return nativeUnavailableOn(runtime.GOOS, runtime.GOARCH) }
	installedPHPVersionsFn = phpDet.ListInstalled
)

// phpRuntimeFromArgs parses the command's argv. Returns (mode, show, err);
// show true means "no argument, print the current value".
func phpRuntimeFromArgs(args []string) (mode string, show bool, err error) {
	if len(args) == 0 {
		return "", true, nil
	}
	switch args[0] {
	case config.PHPRuntimeContainer, config.PHPRuntimeNative:
		return args[0], false, nil
	}
	return "", false, fmt.Errorf("unknown runtime %q, expected %q or %q",
		args[0], config.PHPRuntimeContainer, config.PHPRuntimeNative)
}

// switchableToNative reports whether the runtime switch should touch a site.
// A paused or ignored site keeps the vhost it already has, and rewriting one is
// worse than useless: an ignored secured site whose certificate has been
// removed still renders an SSL vhost naming it, and a certificate nginx cannot
// load fails the entire configuration, so every reload afterwards is refused
// and every other site goes on serving the runtime it was on before.
func switchableToNative(s *config.Site) bool {
	if s.Paused || s.Ignored {
		return false
	}
	return s.ServedNatively(config.PHPRuntimeNative)
}

// fpmUnitsFor maps PHP versions to the shared FPM container units serving them.
func fpmUnitsFor(versions []string) []string {
	units := make([]string, 0, len(versions))
	for _, v := range versions {
		units = append(units, podman.FPMUnitName(v))
	}
	return units
}

// ApplyPHPRuntime switches the install between the container and native PHP
// runtimes. Exported so the dashboard and the CLI drive one path: the switch
// touches every site's .env, vhost, framework cache and workers, and two
// implementations of that would drift.
func ApplyPHPRuntime(mode string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	// Containers stop, env is rewritten and the other runtime starts, so for a
	// few seconds nginx, the pools and every site are legitimately down. The
	// marker lets the status surfaces say "switching" instead of "broken".
	_ = config.MarkRuntimeSwitch()
	defer config.ClearRuntimeSwitch()
	versions, _ := phpDet.ListInstalled()

	// Preflight before writing anything: a missing binary must refuse the
	// switch, not leave the install pointed at a runtime nothing serves.
	if mode == config.PHPRuntimeNative {
		if reg, err := config.LoadSites(); err == nil && reg != nil {
			if msg := unsupportedSitesMessage(reg.Sites); msg != "" {
				return errors.New(msg)
			}
		}
		ensureHostNginx(os.Stdout)
		pins := &pinnedTools{}
		for _, v := range nativeVersionsWanted() {
			if err := ensureNativePHPInstalled(pins, v, os.Stdout); err != nil {
				return err
			}
			if err := nativephp.EnsureInstalled(v, nativephp.FPMBinaryPath(v)); err != nil {
				return err
			}
			if err := nativephp.EnsureInstalled(v, nativephp.BinaryPath(v)); err != nil {
				return err
			}
		}
	}

	sites, err := config.LoadSites()
	if err != nil {
		return err
	}
	// Workers hold their env in memory and never re-read it, so they stop
	// before the rewrite and start again after it in the new shape.
	stopped := map[string][]string{}
	for i := range sites.Sites {
		s := &sites.Sites[i]
		if !switchableToNative(s) {
			continue // never used the shared FPM container, or is not being served
		}
		running := collectRunningWorkers(s)
		stopped[s.Name] = running
		for _, w := range running {
			WorkerStopForSite(s.Name, s.Path, w) //nolint:errcheck
		}
	}

	cfg.PHP.Runtime = mode
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	// nginx's new home is decided before any vhost is rewritten for it.
	moved, err := webserver.Place()
	if err != nil {
		return fmt.Errorf("placing nginx: %w", err)
	}

	// The xdebug ini belongs to the runtime that wrote it, and only a switch
	// changes which one that is.
	if err := podman.RefreshXdebugInis(xdebugRefreshVersions(mode, versions)); err != nil {
		feedback.Warn("refreshing the xdebug ini: %v (xdebug may not load)", err)
	}

	if mode == config.PHPRuntimeNative {
		// The native runtime auto-prepends the bridge from its host copy, which
		// is otherwise only refreshed when an image is built. A stale copy here
		// would be a version of the bridge that cannot find its own assets.
		if err := podman.EnsureDumpAssets(); err != nil {
			feedback.Warn("refreshing the debug bridge assets: %v (dump()/dd() may not capture)", err)
		}
		for _, v := range nativeVersionsWanted() {
			if err := nativephp.Ensure(v); err != nil {
				return fmt.Errorf("starting native php-fpm %s: %w", v, err)
			}
			if _, err := nativephp.EnsureShim(v, nativephp.BinaryPath(v)); err != nil {
				return fmt.Errorf("linking the native php %s: %w", v, err)
			}
		}
	} else {
		// The images may not be there any more: moving PHP to the host offers
		// to remove them, and a version with no image has nothing to start, so
		// coming back would leave every site it serves answering 502. Build or
		// pull whatever is missing before anything is asked to start.
		ensureFPMQuadlets(versionsInUse(versions))
		for _, unit := range fpmUnitsFor(versions) {
			if err := podman.StartUnit(unit); err != nil {
				feedback.Warn("starting %s: %v", unit, err)
			}
		}
	}

	var moving []config.Site
	for i := range sites.Sites {
		if switchableToNative(&sites.Sites[i]) {
			moving = append(moving, sites.Sites[i])
		}
	}
	announceEnvSweep(moving)
	for i := range sites.Sites {
		s := &sites.Sites[i]
		if !switchableToNative(s) {
			continue
		}
		if err := regenerateSiteVhost(s); err != nil {
			feedback.Warn("regenerating the vhost for %s: %v", s.Name, err)
		}
		regenerateWorktreeVhosts(s)
		rerunSiteEnv(s.Path)
		startWorkersForSite(s, stopped[s.Name], s.PHPVersion)
	}
	regenerateProfilerVhost()
	// nginx that moved with PHP started on the new config. Otherwise a refused reload
	// leaves it on the old runtime, every site 502 while the switch reports success,
	// and one unloadable certificate anywhere causes it, so it is said, not swallowed.
	if !moveWebServer(moved, sites.Sites) {
		if err := webserver.Reload(); err != nil {
			feedback.Warn("nginx did not reload (%v); it is still serving the previous runtime, so sites will answer 502 until 'nginx -t' passes", err)
		}
	}

	// Only now is the old runtime idle. Stopping it before nginx was reloaded
	// left every vhost pointing at something that had just gone away, so the
	// whole install answered 502 for the length of the switch.
	if mode == config.PHPRuntimeNative {
		for _, unit := range fpmUnitsFor(versions) {
			if err := podman.StopUnit(unit); err != nil {
				feedback.Warn("stopping %s: %v", unit, err)
			}
		}
	} else {
		for _, v := range versions {
			if err := nativephp.Stop(v); err != nil {
				feedback.Warn("stopping native php-fpm %s: %v", v, err)
			}
		}
	}
	return nil
}

// xdebugRefreshVersions is which versions a switch rewrites the xdebug ini for:
// the installed ones, plus under native the ones about to be fetched, which
// have no ini of their own yet.
func xdebugRefreshVersions(mode string, installed []string) []string {
	out := slices.Clone(installed)
	if mode != config.PHPRuntimeNative {
		return out
	}
	for _, v := range nativeVersionsWanted() {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// nativeVersionsWanted lists the default PHP version plus the ones the sites
// the native runtime serves run on. Not narrowed by the container images: a
// switch on a machine that never built one has to fetch these, not skip them.
func nativeVersionsWanted() []string {
	var out []string
	// The default backs the php and composer shims even with no site linked.
	if cfg, err := config.LoadGlobal(); err == nil && nativephp.Supported(cfg.PHP.DefaultVersion) {
		out = append(out, cfg.PHP.DefaultVersion)
	}
	sites, err := config.LoadSites()
	if err != nil {
		return out
	}
	for i := range sites.Sites {
		s := &sites.Sites[i]
		if !switchableToNative(s) {
			continue
		}
		for _, v := range siteAndWorktreePHPVersions(s) {
			if nativephp.Supported(v) && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	slices.Sort(out)
	return out
}

// siteAndWorktreePHPVersions is the site's PHP version plus any a worktree of it
// pins in its own .lerd.yaml, since each worktree vhost is written for its own.
func siteAndWorktreePHPVersions(s *config.Site) []string {
	var out []string
	if s.PHPVersion != "" {
		out = append(out, s.PHPVersion)
	}
	wts, _ := switchWorktreesFor(s.Path, s.PrimaryDomain())
	for _, wt := range wts {
		if v := config.WorktreePHPVersion(wt.Path, s.PHPVersion); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// versionsInUse narrows the installed PHP versions to those a site the shared
// FPM container serves actually runs on, so the native runtime is only started
// for versions that need it.
func versionsInUse(installed []string) []string {
	sites, err := config.LoadSites()
	if err != nil {
		return installed
	}
	used := map[string]bool{}
	for i := range sites.Sites {
		s := &sites.Sites[i]
		if !switchableToNative(s) {
			continue
		}
		for _, v := range siteAndWorktreePHPVersions(s) {
			used[v] = true
		}
	}
	out := make([]string, 0, len(used))
	for _, v := range installed {
		if used[v] {
			out = append(out, v)
		}
	}
	return out
}

// regenerateSiteVhost rewrites a site's vhost so nginx fastcgi's at the runtime
// now serving it.
// Seams for the worktree sweep, so the switch can be tested without git or nginx.
var (
	switchWorktreesFor       = gitpkg.DetectWorktrees
	switchWriteWorktreeVhost = webserver.ApplyWorktree
)

// regenerateWorktreeVhosts rewrites the vhost of every worktree a site has. A
// worktree is not a registry entry of its own, so the loop above never reached
// one: after a switch its vhost still named the upstream the other runtime
// served, and every worktree answered 502 while its parent was fine.
func regenerateWorktreeVhosts(s *config.Site) {
	wts, err := switchWorktreesFor(s.Path, s.PrimaryDomain())
	if err != nil {
		return
	}
	for _, wt := range wts {
		php := config.WorktreePHPVersion(wt.Path, s.PHPVersion)
		if err := switchWriteWorktreeVhost(wt.Domain, wt.Path, php, s.PrimaryDomain(), s.Name, wt.Branch, s.Secured); err != nil {
			feedback.Warn("regenerating the vhost for %s: %v", wt.Domain, err)
		}
	}
}

// regenerateProfilerVhost rewrites the SPX dashboard's vhost, which belongs to
// no site, so the sweep over sites never reaches it.
func regenerateProfilerVhost() {
	if err := webserver.EnsureProfiler(); err != nil {
		feedback.Warn("regenerating the profiler vhost: %v (the SPX dashboard may answer 502)", err)
	}
}

func regenerateSiteVhost(s *config.Site) error {
	return webserver.ApplySite(*s)
}

// envCommandFor builds the `lerd env` invocation for a site, rooted at its
// directory. runEnv reads the process working directory, which is the site only
// when a person typed the command there; the daemon serves every site from one
// process, so this re-executes rather than chdir'ing, which would race.
func envCommandFor(dir string) *exec.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "lerd"
	}
	cmd := exec.Command(self, "env", "--"+keepForeignDBFlag)
	cmd.Dir = dir
	return cmd
}

// rerunSiteEnv runs `lerd env` in a site so its .env follows where its PHP now
// runs. A var so a switch can be tested without spawning lerd.
var rerunSiteEnv = func(dir string) {
	runEnvIfManaged(dir, func() error {
		if out, err := envCommandFor(dir).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	})
}

// unsupportedSitesMessage lists the sites and worktrees whose PHP has no native runtime, or
// "" when every site could move. The switch is install-wide, so naming them all
// at once turns a run of failed attempts into one decision.
func unsupportedSitesMessage(sites []config.Site) string {
	var blocked []string
	seen := map[string]bool{}
	add := func(entry string) {
		if !seen[entry] {
			seen[entry] = true
			blocked = append(blocked, entry)
		}
	}
	for i := range sites {
		s := &sites[i]
		if !switchableToNative(s) {
			continue
		}
		if !nativephp.Supported(s.PHPVersion) {
			add(s.Name + " (php " + s.PHPVersion + ")")
		}
		// A worktree pinning an older PHP would get a vhost with no listener behind it.
		wts, _ := switchWorktreesFor(s.Path, s.PrimaryDomain())
		for _, wt := range wts {
			if v := config.WorktreePHPVersion(wt.Path, s.PHPVersion); v != s.PHPVersion && !nativephp.Supported(v) {
				add(wt.Domain + " (php " + v + ")")
			}
		}
	}
	if len(blocked) == 0 {
		return ""
	}
	sort.Strings(blocked)
	return fmt.Sprintf("the native runtime needs php %s or newer, and these sites are older: %s.\nMove them up, or keep this install on the container runtime",
		nativephp.MinVersion, strings.Join(blocked, ", "))
}

// startNativeRuntime brings up the host PHP-FPM listeners a native install is
// served by, fetching any wanted version that is missing, the way container
// mode builds the default's image. Does nothing at all in container mode.
func startNativeRuntime() {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.PHPRuntimeMode() != config.PHPRuntimeNative {
		return
	}
	for _, v := range nativeVersionsWanted() {
		if err := bringUpNativePHP(v, os.Stdout); err != nil {
			feedback.Warn("php %s: %v", v, err)
		}
	}
}

// nativeUnavailableOn explains why this machine cannot run the native runtime,
// or returns "" when it can. Builds are published for Apple silicon only: the
// mount boundary costs the most there, and GitHub retires x86_64 macOS runners
// in August 2027 anyway. An Intel Mac has no binary to fetch, which is a
// different thing from one that has not been downloaded yet, so it needs
// different words.
func nativeUnavailableOn(goos, goarch string) string {
	if goos != "darwin" {
		return "the native runtime is macOS only; elsewhere your project already shares a filesystem with PHP"
	}
	_ = goarch // both macOS architectures have builds
	return ""
}
