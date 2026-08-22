package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/geodro/lerd/internal/podman"
)

// validateWorkerUnitFields refuses any value that would break out of its line
// in a generated unit. Every one of these is interpolated into a single-line
// directive, and a project's .lerd.yaml can set the worker ones, so checking
// only the command left label, restart and schedule free to open a [Service]
// section of their own and add an ExecStartPre systemd would run. Checking the
// whole set at the generation boundary makes that structural rather than a
// property of whichever field someone remembered.
func validateWorkerUnitFields(unitName string, fields map[string]string) error {
	// Sorted so the same bad unit always names the same field first.
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if config.ContainsUnitInjectionChars(fields[name]) {
			return fmt.Errorf("worker unit %q: %s must not contain newline or NUL", unitName, name)
		}
	}
	return nil
}

// workerStartPreflight gates a WorkerStartForSite call on the framework's
// declared dependency rules. Two checks, in order:
//
//   - Check (Composer / File): the rule must MATCH for the worker to be
//     eligible. Used to gate optional packages — e.g. reverb's
//     `Composer: "laravel/reverb"` so a site without the package doesn't
//     try to launch `php artisan reverb:start`, which fails with
//     "There are no commands defined in the 'reverb' namespace".
//
//   - ExcludeCheck (Composer / File): the rule must NOT match. Used to
//     hide a worker when a superseding package is present — e.g. queue's
//     `ExcludeCheck: laravel/horizon` so we don't run `queue:work` on a
//     site where horizon owns queue management.
//
// Failures here return a typed-message error that callers (CLI handlers,
// the dashboard, the self-heal watcher) can surface verbatim. The
// watcher's per-unit cooldown then prevents thrashing on a permanent
// failure.
func workerStartPreflight(sitePath, workerName string, w config.FrameworkWorker) error {
	// Every one of these reaches a line of the generated unit, and a cloned
	// repo's .lerd.yaml custom_workers entry can set them all. The unit writer
	// checks the same set again at its own boundary; this one is here so a
	// start reports the offending field rather than failing later.
	if err := validateWorkerUnitFields(workerName, map[string]string{
		"command":        w.Command,
		"reload command": w.ReloadCommand,
		"label":          w.Label,
		"restart":        w.Restart,
		"schedule":       w.Schedule,
	}); err != nil {
		return err
	}
	if msg := hostWorkerNoNodeMsg(workerName, sitePath, w); msg != "" {
		return errors.New(msg)
	}
	if msg := missingRequiredServiceMsg(workerName, sitePath, w); msg != "" {
		return errors.New(msg)
	}
	if w.Check != nil && !config.MatchesRule(sitePath, *w.Check) {
		if msg := hostWorkerNotReadyMsg(workerName, sitePath, w); msg != "" {
			return errors.New(msg)
		}
		return fmt.Errorf(
			"worker %q skipped: required dependency not satisfied (rule: %s)",
			workerName, describeRule(*w.Check))
	}
	if w.ExcludeCheck != nil && config.MatchesRule(sitePath, *w.ExcludeCheck) {
		return fmt.Errorf(
			"worker %q skipped: superseded by another package on this site (rule: %s)",
			workerName, describeRule(*w.ExcludeCheck))
	}
	return nil
}

// missingRequiredServiceMsg returns an actionable message when the worker
// declares a service it needs and that service isn't running. A Laravel queue
// worker on QUEUE_CONNECTION=redis is the case this exists for: without the
// gate it boots and dies on a PHP "getaddrinfo for lerd-redis failed", which
// says nothing about what to start. The requirement and the env condition that
// scopes it both come from the definition; core only reads them.
func missingRequiredServiceMsg(workerName, sitePath string, w config.FrameworkWorker) string {
	name := requiredServiceFor(sitePath, w)
	if name == "" {
		return ""
	}
	if running, _ := podman.ContainerRunning("lerd-" + name); running {
		return ""
	}
	return fmt.Sprintf("worker %q needs the %s service, which is not running\nStart it first: lerd services start %s", workerName, name, name)
}

// requiredServiceFor returns the service the worker needs on this site, or ""
// when it declares none or its env condition doesn't hold here.
func requiredServiceFor(sitePath string, w config.FrameworkWorker) string {
	if w.RequiresService == nil || w.RequiresService.Name == "" {
		return ""
	}
	if cond := w.RequiresService.WhenEnv; cond != "" {
		key, want, ok := strings.Cut(cond, "=")
		if !ok || envfile.ReadKey(filepath.Join(sitePath, ".env"), key) != want {
			return ""
		}
	}
	return w.RequiresService.Name
}

// hostWorkerNotReadyMsg returns an actionable message for a host worker (vite et
// al) that can't start because its dependency check fails. On a node project the
// cause is almost always JS deps not yet installed on the host, so it names the
// remedy; `lerd setup` is used rather than a bare `npm ci` so the advice holds
// for bun/yarn/pnpm projects too. Returns "" when the worker isn't a host node
// worker, so callers keep their own generic dependency message.
func hostWorkerNotReadyMsg(workerName, sitePath string, w config.FrameworkWorker) string {
	if !w.Host || !isNodeProject(sitePath) {
		return ""
	}
	return fmt.Sprintf("%s worker not started: JS dependencies are not installed. Run `lerd setup` to install them, then `lerd worker start %s`.", workerName, workerName)
}

// hostWorkerNoNodeMsg returns an actionable message for a host node worker
// whose command needs the Node toolchain while Node is unmanaged and neither a
// system node/npm nor bun can be resolved anywhere. Without the gate the unit
// would crash-loop on `npm: command not found`. Returns "" when the worker
// isn't affected (managed Node, bun available, or a non-Node command).
func hostWorkerNoNodeMsg(workerName, sitePath string, w config.FrameworkWorker) string {
	if !w.Host || !isNodeProject(sitePath) || lerdManagesNode() {
		return ""
	}
	if !nodeDet.CommandUsesNode(w.Command) || bunRunnerFor(sitePath, false) != "" {
		return ""
	}
	if len(nodeDet.SystemNodeBinDirs()) > 0 {
		return ""
	}
	return fmt.Sprintf("%s worker not started: %v", workerName, errNoUsableNode())
}

// errNoUsableNode is the shared no-Node error for the preflight gate and the
// host-worker unit generators (the generators also hit it on the boot restore
// path, which never runs the preflight).
func errNoUsableNode() error {
	return errors.New("lerd is not managing Node.js and no node/npm (or bun) could be found on this system. Install Node.js, or run `lerd install` to let lerd manage it")
}

// describeRule renders a FrameworkRule for an end-user error message.
// Keeps the message short — the user only needs to know which dependency
// triggered the gate, not the full rule shape.
func describeRule(r config.FrameworkRule) string {
	switch {
	case r.Composer != "":
		return "composer package " + r.Composer
	case r.File != "":
		return "file " + r.File
	default:
		return "(empty rule)"
	}
}
