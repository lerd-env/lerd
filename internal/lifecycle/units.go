package lifecycle

import (
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	phpPkg "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
)

// CoreUnits returns the container units managed by lerd start/stop.
// Does not include lerd-ui or lerd-watcher — those are added separately in runStart.
// The configured default PHP version is ALWAYS included so the `php`, `composer`,
// and `laravel new` shims have a working FPM container even on a fresh install
// with zero registered sites. Other installed versions are only started when
// at least one site references them; unused versions are left stopped.
// FPMContainersWanted reports whether this install is served by the shared FPM
// containers. False under the native runtime, where PHP runs on the host and
// starting them would undo the teardown the switch performed.
func FPMContainersWanted() bool {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return true
	}
	return cfg.PHPRuntimeMode() != config.PHPRuntimeNative
}

func CoreUnits() []string {
	cfg, _ := config.LoadGlobal()
	units := []string{"lerd-nginx"}
	if cfg == nil || cfg.DNS.Enabled {
		units = append([]string{"lerd-dns"}, units...)
	}
	// Under the native runtime PHP runs on the host, so the shared FPM
	// containers have nothing to serve and starting them would undo the
	// teardown the runtime switch performed. nginx still serves every site and
	// stays either way.
	if !FPMContainersWanted() {
		return units
	}
	active := ActivePHPVersions()
	if cfg != nil && cfg.PHP.DefaultVersion != "" {
		active[cfg.PHP.DefaultVersion] = true
	}
	versions, _ := phpPkg.ListInstalled()
	for _, v := range versions {
		if !active[v] {
			continue
		}
		short := strings.ReplaceAll(v, ".", "")
		units = append(units, "lerd-php"+short+"-fpm")
	}
	return units
}

// InstalledCustomContainerUnits returns units for per-project custom containers
// and per-site FrankenPHP containers that have a unit file installed (plist on
// macOS, quadlet on Linux). These are started alongside FPM and services.
func InstalledCustomContainerUnits() []string {
	var units []string
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	for _, site := range reg.Sites {
		if site.Paused {
			continue
		}
		var unitName string
		switch {
		case site.IsCustomContainer():
			unitName = podman.CustomContainerName(site.Name)
		case site.IsFrankenPHP():
			unitName = podman.FrankenPHPContainerName(site.Name)
		case site.IsCustomFPM():
			unitName = podman.CustomFPMContainerName(site.Name)
		default:
			continue
		}
		// Use the platform-aware check (plist on macOS, .container quadlet on Linux)
		// rather than podman.QuadletInstalled which only checks for .container files
		// and always returns false on macOS where plists are used instead.
		if services.Mgr.ContainerUnitInstalled(unitName) {
			units = append(units, unitName)
		}
	}
	return units
}

// InstalledServiceUnits returns service units that have a unit file installed
// and have not been manually stopped by the user. Used for lerd start.
func InstalledServiceUnits() []string {
	var units []string
	for _, svc := range config.DefaultPresetNames() {
		if services.Mgr.ContainerUnitInstalled("lerd-"+svc) && !config.ServiceIsPaused(svc) {
			units = append(units, "lerd-"+svc)
		}
	}
	customs, _ := config.ListCustomServices()
	for _, svc := range customs {
		if services.Mgr.ContainerUnitInstalled("lerd-"+svc.Name) && !config.ServiceIsPaused(svc.Name) {
			units = append(units, "lerd-"+svc.Name)
		}
	}
	return units
}

// AllInstalledServiceUnits returns all service units that have a unit file
// installed, regardless of paused state. Used for lerd stop.
func AllInstalledServiceUnits() []string {
	var units []string
	for _, svc := range config.DefaultPresetNames() {
		if services.Mgr.ContainerUnitInstalled("lerd-" + svc) {
			units = append(units, "lerd-"+svc)
		}
	}
	customs, _ := config.ListCustomServices()
	for _, svc := range customs {
		if services.Mgr.ContainerUnitInstalled("lerd-" + svc.Name) {
			units = append(units, "lerd-"+svc.Name)
		}
	}
	return units
}

// RegisteredStripeUnits returns unit names for all lerd-stripe-* service units.
func RegisteredStripeUnits() []string {
	return services.Mgr.ListServiceUnits("lerd-stripe-*")
}

// RegisteredQueueUnits returns unit names for all lerd-queue-* service units
// (i.e. started via `lerd queue:start`).
func RegisteredQueueUnits() []string {
	return services.Mgr.ListServiceUnits("lerd-queue-*")
}

// RegisteredScheduleUnits returns unit names for all lerd-schedule-* service units.
func RegisteredScheduleUnits() []string {
	return services.Mgr.ListServiceUnits("lerd-schedule-*")
}

// RegisteredReverbUnits returns unit names for all lerd-reverb-* service units.
func RegisteredReverbUnits() []string {
	return services.Mgr.ListServiceUnits("lerd-reverb-*")
}

// RegisteredTimerUnits returns names for every lerd-* timer unit on disk,
// each with the explicit `.timer` suffix so callers pass them straight to
// systemctl. These drive scheduled (cron-style) framework workers like
// Laravel <=10's `php artisan schedule:run`.
func RegisteredTimerUnits() []string {
	return services.Mgr.ListTimerUnits("lerd-*")
}

// RegisteredFrameworkWorkerUnits returns lerd-{worker}-{site} unit names for
// every site/worker pair declared in the site registry. Used to make sure
// non-standard workers (horizon, vite-dev, etc.) get started in phase 2 of
// runStart, not just the queue/stripe/schedule/reverb glob.
func RegisteredFrameworkWorkerUnits() []string {
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return nil
	}
	out := make([]string, 0)
	for _, s := range reg.Sites {
		if s.Ignored || s.Paused {
			continue
		}
		proj, err := config.LoadProjectConfig(s.Path)
		if err != nil || proj == nil {
			continue
		}
		for _, w := range proj.Workers {
			if w == "stripe" {
				continue
			}
			out = append(out, "lerd-"+w+"-"+s.Name)
		}
		// Enumerate the dev-server unit unconditionally: this list also drives
		// stop/quit, so a drifted unit must stay visible to be stoppable. The
		// drift guard lives in restoreSiteInfrastructure, which won't write the
		// drifted command, so start can only ever launch the approved one.
		if s.IsHostProxy() && proj.Proxy != nil && proj.Proxy.Command != "" {
			out = append(out, config.HostProxyWorkerUnit(s.Name))
		}
	}
	return out
}

// QuitProcessUnits is the ordered set of host process units `lerd quit` tears
// down after Stop. Unlike `lerd stop`, quit is a full teardown, so it includes
// lerd-dns. lerd-watcher precedes lerd-dns because the watcher is the only
// thing that restarts lerd-dns; stopping it first keeps dns down. skip removes
// units the caller must not stop, for a daemon quitting from inside one of them.
func QuitProcessUnits(skip ...string) []string {
	units := []string{"lerd-ui", "lerd-watcher", "lerd-tray", "lerd-dns"}
	return slices.DeleteFunc(units, func(u string) bool { return slices.Contains(skip, u) })
}

// StopUnitSet returns every unit `lerd stop` tears down. lerd-dns is
// deliberately excluded: the resolver points .test at it until uninstall, so
// it stays up as install-level DNS plumbing (the watcher would restart it).
func StopUnitSet(skip ...string) []string {
	units := append(CoreUnits(), AllInstalledServiceUnits()...)
	units = append(units, InstalledCustomContainerUnits()...)
	units = append(units, RegisteredQueueUnits()...)
	units = append(units, RegisteredStripeUnits()...)
	units = append(units, RegisteredScheduleUnits()...)
	units = append(units, RegisteredReverbUnits()...)
	units = append(units, RegisteredFrameworkWorkerUnits()...)
	// Stop scheduled-worker timers explicitly. Stopping the sibling
	// oneshot .service is a no-op (it isn't running between firings),
	// so without this the timer keeps dispatching after `lerd stop`.
	units = append(units, RegisteredTimerUnits()...)
	return slices.DeleteFunc(units, func(u string) bool {
		return u == "lerd-dns" || slices.Contains(skip, u)
	})
}

// ActivePHPVersions returns the set of PHP versions actually in use by
// non-ignored, non-paused sites, using live disk detection (.php-version file)
// with the stored registry value as fallback.
func ActivePHPVersions() map[string]bool {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	active := make(map[string]bool)
	for _, s := range reg.Sites {
		if s.Ignored {
			continue
		}
		phpMin, phpMax := "", ""
		if s.Framework != "" {
			// A guessed framework definition's PHP range must not constrain the
			// site (a Laravel 6 served by the Laravel 10 def still runs on 7.4),
			// so skip its range and let the real detected version drive which
			// FPM unit CoreUnits starts.
			if fw, fwOk := config.GetFrameworkForDir(s.Framework, s.Path); fwOk && !fw.VersionGuessed {
				phpMin, phpMax = fw.PHP.Min, fw.PHP.Max
			}
		}
		v := phpPkg.DetectVersionClamped(s.Path, phpMin, phpMax, s.PHPVersion)
		if v != "" {
			active[v] = true
		}
	}
	return active
}
