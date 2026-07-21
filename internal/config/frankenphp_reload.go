package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/power"
	"github.com/geodro/lerd/internal/wsl"
)

// WatcherNeedsPolling reports whether a file-change watcher has to poll because
// host filesystem events don't reach the process watching them: always on
// macOS (the runtime runs inside the podman VM) and on WSL2 only for projects
// under a /mnt (9p) mount, where inotify isn't delivered. This is the canonical
// home for the predicate; cli.watcherNeedsPolling delegates here so the Horizon
// and Octane reload paths agree.
func WatcherNeedsPolling(sitePath string) bool {
	if runtime.GOOS == "darwin" {
		return true
	}
	return wsl.IsWSL() && strings.HasPrefix(sitePath, "/mnt/")
}

// watcherPollIntervalMS is how often a polling reload watcher re-stats each
// watched file while the machine is plugged in. chokidar's own default is
// 100ms, which is fine on a local disk and ruinous on a shared one: every
// watched file becomes ten host round trips a second. On macOS that traffic
// crosses virtiofs and is served by the VM process, which is where the cost
// lands (measured at roughly 280 virtiofs requests per second for a single
// site, holding the VM process near 30% CPU on an otherwise idle machine). A
// second between passes keeps reload feeling immediate for a worker that takes
// longer than that to restart anyway.
const watcherPollIntervalMS = 1000

// watcherPollIntervalFor scales the base interval by how much the host has
// asked lerd to hold back. Waking the disk to stat a few hundred files is
// exactly the kind of background work a laptop on battery can do without, and
// Low Power Mode is an explicit request for less of it, so it backs off twice
// as far again.
func watcherPollIntervalFor(state power.State) int {
	switch state {
	case power.LowPower:
		return watcherPollIntervalMS * 4
	case power.Battery:
		return watcherPollIntervalMS * 2
	default:
		return watcherPollIntervalMS
	}
}

// WatcherPollEnv returns the KEY=VALUE environment assignment that loosens a
// polling reload watcher, or an empty string when the watcher is not polling
// and the default event-driven path applies. chokidar honours CHOKIDAR_INTERVAL
// as a global override no matter how polling was switched on, so this tunes the
// vendored Octane and Horizon watcher scripts without patching them.
//
// The value is resolved when the worker's unit is written, so a machine that
// changes power state picks up the new cadence on the next worker start rather
// than immediately: the interval is baked into the running watcher's
// environment and chokidar reads it only once, at startup.
func WatcherPollEnv(sitePath string) string {
	if !WatcherNeedsPolling(sitePath) {
		return ""
	}
	return "CHOKIDAR_INTERVAL=" + strconv.Itoa(watcherPollIntervalFor(power.Current()))
}

// ProjectHasChokidar reports whether the chokidar npm package, required by the
// Octane and Horizon file watchers, is installed in the project. Canonical home;
// cli.projectHasChokidar delegates here.
func ProjectHasChokidar(sitePath string) bool {
	info, err := os.Stat(filepath.Join(sitePath, "node_modules", "chokidar"))
	return err == nil && info.IsDir()
}

// ResolveFrankenPHPWorkerEntrypoint returns the entrypoint to launch inside the
// FrankenPHP container, substituting the watch-enabled variant
// (WorkerReloadEntrypoint) for WorkerEntrypoint when the site is in worker mode,
// the framework declares a watch variant, and the project has opted the "octane"
// worker into reload. On hosts where the container can't observe host filesystem
// events it appends Octane's --poll flag, mirroring how Horizon's
// resolveWorkerCommand handles polling.
//
// When reload is requested but chokidar is absent it silently falls back to the
// standard entrypoint; the enable paths (cli.ApplyOctaneReload and the UI) refuse
// up front when chokidar is missing, so the displayed state never diverges from
// what actually runs.
func (fw *Framework) ResolveFrankenPHPWorkerEntrypoint(sitePath string, worker bool) []string {
	base := fw.FrankenPHPEntrypoint(worker)
	if !worker || fw == nil || fw.FrankenPHP == nil {
		return base
	}
	if len(fw.FrankenPHP.WorkerReloadEntrypoint) == 0 {
		return base
	}
	if !ProjectReloadsWorker(sitePath, "octane") || !ProjectHasChokidar(sitePath) {
		return base
	}
	ep := append([]string(nil), fw.FrankenPHP.WorkerReloadEntrypoint...)
	if WatcherNeedsPolling(sitePath) {
		ep = appendPollFlag(ep)
	}
	return ep
}

// FrankenPHPQuadletSpec resolves the entrypoint and env a site's FrankenPHP
// container should run with, applying the reload-aware worker entrypoint
// (octane:start --watch) when the project opted in. Both the apply path
// (siteops.FinishFrankenPHPLink) and the global install refresh resolve through
// here, so a site's quadlet can't diverge between the two writers.
func (s *Site) FrankenPHPQuadletSpec() (entrypoint []string, env map[string]string) {
	fw, _ := GetFrameworkForDir(s.Framework, s.Path)
	return fw.ResolveFrankenPHPWorkerEntrypoint(s.Path, s.RuntimeWorker), fw.FrankenPHPEnv(s.RuntimeWorker)
}

// appendPollFlag adds Octane's --poll to a resolved watch entrypoint. For the
// `sh -c "<script>"` form used to install pcntl before exec'ing octane, the flag
// must go inside the script (the trailing element); for a bare argv form it is a
// normal trailing argument.
func appendPollFlag(ep []string) []string {
	if len(ep) >= 2 && ep[0] == "sh" && ep[1] == "-c" {
		ep[len(ep)-1] += " --poll"
		return ep
	}
	return append(ep, "--poll")
}
