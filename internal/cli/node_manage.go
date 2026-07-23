package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
	"github.com/spf13/cobra"
)

// NewNodeManageCmd returns the node:manage command, which opts the host into
// lerd-managed Node.js (version-manager shims) after the fact, for users who
// declined at `lerd install` time.
func NewNodeManageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node:manage",
		Short: "Let lerd manage Node.js (install shims and a default version)",
		Args:  cobra.NoArgs,
		RunE:  runNodeManage,
	}
}

func runNodeManage(_ *cobra.Command, _ []string) error {
	if lerdManagesNode() {
		feedback.Begin()
		feedback.Line("lerd is already managing Node.js")
		return nil
	}
	mgr := nodeDet.Active()
	if !mgr.Available() {
		return fmt.Errorf("%s not found — run 'lerd install' first", mgr.Name())
	}
	feedback.Begin()
	step := feedback.Start("enabling lerd-managed Node")
	if err := addShellShims(true); err != nil {
		step.Fail(err)
		return fmt.Errorf("writing shims: %w", err)
	}
	step.OK("")
	ensureDefaultNode()
	persistNodeManaged(true)
	// Host workers (Vite etc.) were generated to run directly or via bun while
	// Node was unmanaged; rewrite them so they route through the manager again.
	regenerateHostWorkers()
	feedback.Done("lerd is now managing Node.js")
	feedback.Note("pin a version per project with `lerd isolate:node <v>`")
	return nil
}

// NewNodeManagerCmd returns the node:manager command, which reports or switches
// the Node version manager lerd drives ("fnm" or "nvm"). Switching persists the
// choice and, when lerd is managing Node, rewrites the shims, ensures a default
// version, and re-syncs host workers so the new manager takes effect at once.
func NewNodeManagerCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "node:manager [fnm|nvm]",
		Short:     "Show or switch the Node version manager lerd drives",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"fnm", "nvm"},
		RunE:      runNodeSetManager,
	}
}

func runNodeSetManager(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return fmt.Errorf("loading config: %w", err)
	}
	current := cfg.NodeManager()

	// No argument: report the active manager.
	if len(args) == 0 {
		feedback.Begin()
		feedback.Line("Node version manager: " + feedback.Val(current))
		return nil
	}

	target := args[0]
	if target != "fnm" && target != "nvm" {
		return fmt.Errorf("unknown manager %q — use 'fnm' or 'nvm'", target)
	}
	if target == current {
		feedback.Begin()
		feedback.Line("already using " + feedback.Val(target))
		return nil
	}
	if !nodeDet.ManagerByName(target).Available() {
		if target == "nvm" {
			return fmt.Errorf("nvm not found — install it first (https://github.com/nvm-sh/nvm)")
		}
		return fmt.Errorf("fnm not found — run 'lerd install' to set it up")
	}

	cfg.SetNodeManager(target)
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	feedback.Begin()
	// Only rewrite shims/workers when lerd is actually managing Node; otherwise
	// the choice is just persisted and applies whenever management is enabled.
	if lerdManagesNode() {
		step := feedback.Start("updating Node PATH shims for " + target)
		if err := addShellShims(true); err != nil {
			step.Fail(err)
			return fmt.Errorf("writing shims: %w", err)
		}
		step.OK("")
		ensureDefaultNode()
		regenerateHostWorkers()
	}
	feedback.Done("Node version manager set to " + feedback.Val(target))
	return nil
}

// NewNodeUnmanageCmd returns the node:unmanage command, which removes lerd's
// node shims and, when lerd owns the version manager (fnm), the Node binaries it
// installed — leaving a clean system so the user can rely on bun or their own
// system Node.
func NewNodeUnmanageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node:unmanage",
		Short: "Stop managing Node.js: remove lerd's node shims and fnm-installed versions",
		Args:  cobra.NoArgs,
		RunE:  runNodeUnmanage,
	}
}

// persistNodeManaged records the Node-management choice in config.yaml so it
// survives `lerd update`, which otherwise recomputes the intent and would
// re-add the shims a node:unmanage removed. Best-effort: a save failure is
// warned, not fatal, since the shims themselves already reflect the choice.
func persistNodeManaged(managed bool) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return
	}
	if v, set := cfg.NodeManagedPref(); set && v == managed {
		return
	}
	cfg.SetNodeManaged(managed)
	if err := config.SaveGlobal(cfg); err != nil {
		feedback.Warn("persist Node-management choice: %v", err)
	}
}

var fnmVersionRe = regexp.MustCompile(`v\d+\.\d+\.\d+`)

func runNodeUnmanage(_ *cobra.Command, _ []string) error {
	// Uninstall every fnm-managed Node version so no stale binaries linger, but
	// only when lerd owns the manager. fnm is bundled by lerd, so its versions
	// are lerd's to remove; a user-installed nvm and its versions belong to the
	// user and must be left untouched — we only drop lerd's shims for those.
	if nodeDet.Active().Name() == "fnm" {
		fnmPath := filepath.Join(config.BinDir(), "fnm")
		if _, err := os.Stat(fnmPath); err == nil {
			if out, err := exec.Command(fnmPath, "list").CombinedOutput(); err == nil {
				seen := map[string]bool{}
				for _, v := range fnmVersionRe.FindAllString(string(out), -1) {
					if seen[v] {
						continue
					}
					seen[v] = true
					if uo, uerr := exec.Command(fnmPath, "uninstall", v).CombinedOutput(); uerr != nil {
						feedback.Warn("fnm uninstall %s: %s", v, string(uo))
					} else {
						feedback.Note("removed Node " + v)
					}
				}
			}
		}
	}

	// Remove the node/npm/npx shims (addShellShims(false) deletes them), so the
	// user's system node/npm/npx are no longer masked on PATH.
	if err := addShellShims(false); err != nil {
		return fmt.Errorf("removing shims: %w", err)
	}

	// Existing host worker units still reference the manager's `exec … -- npm …`,
	// which now has no Node to run; rewrite them so they use bun (when present)
	// or the user's system Node directly.
	persistNodeManaged(false)
	feedback.Begin()
	regen := feedback.Start("regenerating host worker units")
	regenerateHostWorkers()
	regen.OK("")

	feedback.Done("lerd is no longer managing Node.js")
	if nodeDet.BunPath() != "" {
		feedback.Note("bun is installed, so JS host workers (e.g. Vite) now run through bun")
	} else {
		feedback.Note("JS host workers (e.g. Vite) now use your system Node; install bun or a system Node if you have neither")
	}
	feedback.Note("re-enable lerd-managed Node with `lerd node:manage`")
	return nil
}

// regenerateHostWorkers rewrites and restarts every registered site's active
// host worker units (Vite and other host:true workers) so they pick up the
// current JS-runtime decision after node:manage / node:unmanage flips what Node
// is available. Best-effort: failures are warned, not fatal.
// hostWorkerHeader, when set, prints the "Starting host workers" section header
// exactly once, just before the first worker is actually started. It is wired
// up only by the install-time regenerateHostWorkers sweep, so the per-site
// dashboard runtime toggle (which also calls RegenerateHostWorkersForSite)
// doesn't emit a header, and a sweep that starts nothing leaves no orphan.
var hostWorkerHeader func()

func regenerateHostWorkers() {
	reg, err := config.LoadSites()
	if err != nil {
		return
	}
	var once sync.Once
	hostWorkerHeader = func() { once.Do(func() { feedback.Header("Re-syncing host workers") }) }
	defer func() { hostWorkerHeader = nil }()
	for _, s := range reg.Sites {
		RegenerateHostWorkersForSite(s)
	}
}

// RegenerateHostWorkersForSite rewrites and restarts (only when changed) the
// host worker units of one site, so a JS-runtime change (e.g. flipping
// js_runtime to bun from the dashboard) takes effect on its Vite/dev worker
// without a manual restart. Best-effort. Paused/ignored sites are skipped so a
// runtime toggle does not resurrect a worker the user stopped.
func RegenerateHostWorkersForSite(s config.Site) {
	if s.Paused || s.Ignored {
		return
	}
	proj, _ := config.LoadProjectConfig(s.Path)
	if proj == nil {
		return
	}
	// Host-proxy sites run their dev command (`env PORT=N npm run ...`) as a
	// host worker too but have no framework, so handle them directly — they
	// are exactly the npm-on-host commands that should switch to bun.
	if s.IsHostProxy() {
		if proj.Proxy != nil {
			if w, ok := hostProxyWorker(proj.Proxy); ok {
				regenerateWorkerUnit(s.Name, s.Path, "", hostProxyWorkerName, w, hostProxyWorkerUnit(s.Name))
			}
		}
		return
	}
	fw, ok := config.GetFrameworkForDir(s.Framework, s.Path)
	if !ok || fw.Workers == nil {
		return
	}
	phpVersion := s.PHPVersion
	if phpVersion == "" {
		if cfg, _ := config.LoadGlobal(); cfg != nil {
			phpVersion = cfg.PHP.DefaultVersion
		}
	}
	// Iterate the framework's host workers directly, not proj.Workers:
	// some host workers (Vite is replaces_build/per_worktree) are enabled
	// via the build flow and never persisted to the saved workers list.
	for w, wDef := range fw.Workers {
		if !wDef.Host {
			continue
		}
		// Don't resurrect a host worker the idle engine has suspended. Restarting
		// it here also runs ClearIdleSuspendOnStart, dropping it from the suspended
		// list, so the engine can no longer see it running and it stays up forever
		// on an idle site. The worktree path below already filters via
		// worktreeWorkersToStart; this is the main-site equivalent.
		if containsString(s.IdleSuspendedWorkers, w) {
			continue
		}
		regenerateWorkerUnit(s.Name, s.Path, phpVersion, w, wDef, "lerd-"+w+"-"+s.Name)
	}
	// A site's git worktrees run their own per-worktree host workers (e.g. Vite)
	// under suffixed units; regenerate those too so a runtime toggle reaches a
	// worktree's dev server instead of leaving it on the old runtime.
	regenerateWorktreeHostWorkers(&s, fw, phpVersion)
}

// regenerateWorktreeHostWorkers rewrites and restarts (only when changed) the
// per-worktree host worker units of a site, the worktree analogue of the main
// loop in RegenerateHostWorkersForSite. Idle-suspended worktree workers are left
// down (regenerateWorkerUnit also skips any unit that isn't enabled).
func regenerateWorktreeHostWorkers(site *config.Site, fw *config.Framework, phpVersion string) {
	wts, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return
	}
	for _, wt := range wts {
		if wt.Path == site.Path {
			continue // the main checkout, handled by the caller
		}
		wtBase := config.WorktreeUnitSlug(filepath.Base(wt.Path))
		names := worktreeWorkersToStart(site, wtBase, OptedInHostWorkers(site, wt.Path))
		for _, name := range names {
			wDef, ok := fw.Workers[name]
			if !ok {
				continue
			}
			regenerateWorkerUnit(site.Name, wt.Path, phpVersion, name, wDef, "lerd-"+name+"-"+site.Name+"-"+wtBase)
		}
	}
}

// regenerateWorkerUnit rewrites one enabled host worker unit and restarts it
// only when its content actually changed, so a re-sync doesn't disrupt workers
// already on the right runtime. persist=false keeps .lerd.yaml untouched (Vite
// is a build-replacer that's intentionally not persisted). Best-effort.
func regenerateWorkerUnit(siteName, sitePath, phpVersion, workerName string, wDef config.FrameworkWorker, unitName string) {
	if !services.Mgr.IsEnabled(unitName) {
		return
	}
	// Snapshot the unit before rewriting so we only restart it when its
	// ExecStart actually changed. On macOS the unit is a launchd plist
	// elsewhere, so before is empty and we always fall through to restart.
	// Rewrite the unit quietly first so we can tell whether the runtime actually
	// changed, without starting or announcing it. startRestoredServices already
	// started this worker during install (or it's a build-replacer like Vite that
	// isn't persisted), so an unchanged rewrite should stay silent rather than
	// print a second time under its own header.
	unitPath := filepath.Join(config.SystemdUserDir(), unitName+".service")
	before, _ := os.ReadFile(unitPath)
	restoreWorker(siteName, sitePath, phpVersion, workerName, wDef)
	after, _ := os.ReadFile(unitPath)
	if len(before) > 0 && string(before) == string(after) {
		// Linux, genuinely unchanged: StartUnit no-ops on an active unit, so just
		// make sure it's running without announcing it again.
		_ = podman.StartUnit(unitName)
		return
	}
	// The unit changed, or can't be diffed (macOS host workers are launchd plists,
	// not .service files, so both reads come back empty): announce under the
	// host-workers header, then do a full platform-correct (re)start.
	// WorkerStartForSite owns the macOS launchd lifecycle, clears stale
	// idle-suspend state, and regenerates the proxy vhost; the reload+restart
	// afterward bounces a running Linux worker onto the new ExecStart, which
	// StartUnit alone would not.
	if hostWorkerHeader != nil {
		hostWorkerHeader()
	}
	if err := WorkerStartForSite(siteName, sitePath, phpVersion, workerName, wDef, false); err != nil {
		feedback.Warn("re-syncing %s: %v", unitName, err)
		return
	}
	_ = podman.DaemonReload()
	podman.ResetFailedUnit(unitName)
	_ = podman.RestartUnit(unitName)
}
