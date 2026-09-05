package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/nginx"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// NewPHPRuntimeCmd returns the `lerd php:runtime` command.
func NewPHPRuntimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "php:runtime [container|native]",
		Short: "Show or set where PHP runs (macOS only)",
		Long: `Show or set where PHP runs for every site the shared FPM container serves.

  container  PHP-FPM, the CLI and the workers run in containers. Default, and
             the only mode on Linux.

  native     they run on the macOS host instead. Your project is bind-mounted
             into the Podman VM, so a containerised PHP crosses that boundary
             for every file it reads; running on the host removes it.
             Requires PHP 8.1 or newer. Beta.

No argument prints the current value. The setting is install-wide: the FPM
container is shared by every site on a PHP version, so sites cannot be moved
one at a time.

Switching rewrites each site's .env for the service addresses the new runtime
can reach, regenerates the vhosts, drops the framework config caches, and
restarts the workers. Going native also stops the shared FPM containers, which
have nothing left to serve.

FrankenPHP sites, custom containers and host-proxy sites are unaffected: they
never used the shared FPM container.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			mode, show, err := phpRuntimeFromArgs(args)
			if err != nil {
				return err
			}
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if show {
				fmt.Printf("PHP runtime: %s\n", cfg.PHPRuntimeMode())
				if runtime.GOOS != "darwin" {
					fmt.Println("  (Linux runs PHP in containers; the native runtime is macOS only.)")
				}
				return nil
			}
			if reason := nativeUnavailableOn(runtime.GOOS, runtime.GOARCH); mode == config.PHPRuntimeNative && reason != "" {
				return errors.New(reason)
			}
			prev := cfg.PHPRuntimeMode()
			feedback.Begin()
			if prev == mode {
				feedback.Line("PHP runtime already " + mode)
				return nil
			}
			if err := ApplyPHPRuntime(mode); err != nil {
				return err
			}
			feedback.Done("PHP runtime set to " + feedback.Val(mode) + " (was " + prev + ")")
			return nil
		},
	}
}

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
	versions, _ := phpDet.ListInstalled()

	// Preflight before writing anything: a missing binary must refuse the
	// switch, not leave the install pointed at a runtime nothing serves.
	if mode == config.PHPRuntimeNative {
		if reg, err := config.LoadSites(); err == nil && reg != nil {
			if msg := unsupportedSitesMessage(reg.Sites); msg != "" {
				return errors.New(msg)
			}
		}
		pins := &pinnedTools{}
		for _, v := range versionsInUse(versions) {
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
		if !s.ServedNatively(config.PHPRuntimeNative) {
			continue // never used the shared FPM container
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

	if mode == config.PHPRuntimeNative {
		// The native runtime auto-prepends the bridge from its host copy, which
		// is otherwise only refreshed when an image is built. A stale copy here
		// would be a version of the bridge that cannot find its own assets.
		if err := podman.EnsureDumpAssets(); err != nil {
			feedback.Warn("refreshing the debug bridge assets: %v (dump()/dd() may not capture)", err)
		}
		for _, v := range versionsInUse(versions) {
			if err := nativephp.Ensure(v); err != nil {
				return fmt.Errorf("starting native php-fpm %s: %w", v, err)
			}
			if _, err := nativephp.EnsureShim(v, nativephp.BinaryPath(v)); err != nil {
				return fmt.Errorf("linking the native php %s: %w", v, err)
			}
		}
	} else {
		for _, unit := range fpmUnitsFor(versions) {
			if err := podman.StartUnit(unit); err != nil {
				feedback.Warn("starting %s: %v", unit, err)
			}
		}
	}

	for i := range sites.Sites {
		s := &sites.Sites[i]
		if !s.ServedNatively(config.PHPRuntimeNative) {
			continue
		}
		if err := regenerateSiteVhost(s); err != nil {
			feedback.Warn("regenerating the vhost for %s: %v", s.Name, err)
		}
		runEnvIfManaged(s.Path, func() error {
			if out, err := envCommandFor(s.Path).CombinedOutput(); err != nil {
				return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		})
		startWorkersForSite(s, stopped[s.Name], s.PHPVersion)
	}
	_ = nginx.Reload()

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
		if s.ServedNatively(config.PHPRuntimeNative) && s.PHPVersion != "" {
			used[s.PHPVersion] = true
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
func regenerateSiteVhost(s *config.Site) error {
	if s.Secured {
		if err := nginx.GenerateSSLVhost(*s, s.PHPVersion); err != nil {
			return err
		}
		return nginx.InstallSSLVhost(s.PrimaryDomain())
	}
	return nginx.GenerateVhost(*s, s.PHPVersion)
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
	cmd := exec.Command(self, "env")
	cmd.Dir = dir
	return cmd
}

// unsupportedSitesMessage lists the sites whose PHP has no native runtime, or
// "" when every site could move. The switch is install-wide, so naming them all
// at once turns a run of failed attempts into one decision.
func unsupportedSitesMessage(sites []config.Site) string {
	var blocked []string
	seen := map[string]bool{}
	for i := range sites {
		s := &sites[i]
		if !s.ServedNatively(config.PHPRuntimeNative) || nativephp.Supported(s.PHPVersion) {
			continue
		}
		entry := s.Name + " (php " + s.PHPVersion + ")"
		if seen[entry] {
			continue
		}
		seen[entry] = true
		blocked = append(blocked, entry)
	}
	if len(blocked) == 0 {
		return ""
	}
	sort.Strings(blocked)
	return fmt.Sprintf("the native runtime needs php %s or newer, and these sites are older: %s.\nMove them up, or keep this install on the container runtime",
		nativephp.MinVersion, strings.Join(blocked, ", "))
}

// startNativeRuntime brings up the host PHP-FPM listeners a native install is
// served by, and does nothing at all in container mode. Called from the start
// path, where the FPM containers are absent by design.
func startNativeRuntime() {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.PHPRuntimeMode() != config.PHPRuntimeNative {
		return
	}
	versions, _ := phpDet.ListInstalled()
	for _, v := range versionsInUse(versions) {
		if err := nativephp.Ensure(v); err != nil {
			feedback.Warn("starting native php-fpm %s: %v", v, err)
			continue
		}
		if _, err := nativephp.EnsureShim(v, nativephp.BinaryPath(v)); err != nil {
			feedback.Warn("linking the native php %s: %v", v, err)
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
