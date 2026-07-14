// Package workerheal detects and recovers worker units stuck in systemd's
// "failed" state. The detector is deliberately cheap — it walks the existing
// batched unit-state cache shared with the dashboard, so polling stays free
// even on busy installs. The healer is a single primitive: reset-failed +
// start. It never writes .lerd.yaml or rewrites unit files; that belongs to
// `lerd worker add/remove/start/stop` and `lerd init`.
package workerheal

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/siteinfo"
)

// UnhealthyWorker is a single failing/stuck worker unit.
type UnhealthyWorker struct {
	Site      string `json:"site"`
	Worker    string `json:"worker"`
	Unit      string `json:"unit"`
	State     string `json:"state"` // "failed" | "expected-but-stopped" | "unreachable" (process up, server not accepting)
	LastError string `json:"last_error,omitempty"`
}

// Event is one line in the streaming heal report. Dashboard, MCP, and TUI
// all consume these so progress is visible without polling.
type Event struct {
	Phase string `json:"phase"` // "starting" | "healed" | "failed" | "done"
	Site  string `json:"site,omitempty"`
	Unit  string `json:"unit,omitempty"`
	Error string `json:"error,omitempty"`
}

// Result is the aggregate report for non-streaming callers.
type Result struct {
	Healed []UnhealthyWorker `json:"healed"`
	Failed []Failure         `json:"failed"`
}

// Failure is one heal attempt that errored.
type Failure struct {
	Worker UnhealthyWorker `json:"worker"`
	Err    string          `json:"error"`
}

// nonWorkerPerSitePrefixes lists lerd-<X>-<site> patterns that match a
// registered site suffix but are NOT worker units (per-site containers
// rather than worker processes). Heal must skip these — restarting a
// crashed lerd-fp-myapp via this path is a different operation.
var nonWorkerPerSitePrefixes = map[string]bool{
	"custom": true, // lerd-custom-<site> — per-site custom container
	"fp":     true, // lerd-fp-<site>     — per-site FrankenPHP container
}

// Swappable for tests so the detector can be exercised without touching the
// real systemd unit-state cache or starting real units.
var (
	unitStatesFn      = siteinfo.AllUnitStates
	unitMetaFn        = siteinfo.AllUnitMeta
	unitEnabledFn     = isUnitEnabled
	healFn            = podman.StartUnit
	restartFn         = podman.RestartUnit
	lastErrorFn       = readLastError
	isStoppedFn       = config.IsStopped
	workerReachableFn = defaultWorkerReachable
)

// defaultWorkerReachable probes a running worker that declares a health block.
// probed is false (keep process-only liveness) when the site has no resolvable
// framework or the worker has no health block. The framework is resolved once
// per site by Detect and passed in, so the store is touched at most once per
// site per tick, never per worker.
func defaultWorkerReachable(sitePath string, fw *config.Framework, worker string, activeEnter time.Time) (reachable, probed bool) {
	if fw == nil {
		return false, false
	}
	w, ok := fw.Workers[worker]
	if !ok || w.Health == nil {
		return false, false
	}
	return siteinfo.WorkerServerReachable(sitePath, w.Health, activeEnter)
}

// resolveWorkerUnit maps a unit body to site, worker, and probe path (the worktree
// checkout, else ""). A unit running in a site's own checkout is that site's worker,
// so only a working directory that is not a site path can be a worktree's — and that
// has to be settled before the suffix match, which cannot tell a worktree directory
// named after another registered site from that site's own worker.
func resolveWorkerUnit(body string, sitePaths map[string]string, workingDir string) (site, worker, probePath string) {
	// A container worker sets no WorkingDirectory, so systemd reports the inherited
	// home with a "!" prefix. That is not a checkout.
	if strings.HasPrefix(workingDir, "!") {
		workingDir = ""
	}
	if workingDir != "" && !isSiteCheckout(sitePaths, workingDir) {
		if s, w := resolveWorktreeUnit(body, sitePaths, workingDir); s != "" {
			return s, w, workingDir
		}
	}
	for s := range sitePaths {
		if strings.HasSuffix(body, "-"+s) && len(s) > len(site) {
			site, worker = s, strings.TrimSuffix(body, "-"+s)
		}
	}
	return site, worker, ""
}

func isSiteCheckout(sitePaths map[string]string, dir string) bool {
	for _, p := range sitePaths {
		if p != "" && p == dir {
			return true
		}
	}
	return false
}

// resolveWorktreeUnit resolves a unit against its checkout directory, returning
// empty when the unit is not a worktree's.
func resolveWorktreeUnit(body string, sitePaths map[string]string, workingDir string) (site, worker string) {
	slug := config.WorktreeUnitSlug(filepath.Base(workingDir))
	if slug == "" || !strings.HasSuffix(body, "-"+slug) {
		return "", ""
	}
	core := strings.TrimSuffix(body, "-"+slug)
	for s := range sitePaths {
		if strings.HasSuffix(core, "-"+s) && len(s) > len(site) {
			site, worker = s, strings.TrimSuffix(core, "-"+s)
		}
	}
	if worker == "" {
		return "", ""
	}
	return site, worker
}

// HumanState renders an UnhealthyWorker.State for end-user copy. The machine
// values ("failed", "expected-but-stopped") read awkwardly in a sentence.
func HumanState(state string) string {
	switch state {
	case "expected-but-stopped":
		return "stopped"
	case "":
		return "failed"
	default:
		return state
	}
}

// lastErrorMaxLen caps how many characters of an error line are surfaced.
// Truncated lines keep the dashboard frame small and avoid leaking long
// stack traces over the WS push.
const lastErrorMaxLen = 220

// readLastError returns the last log line emitted for a failed worker unit.
// Best-effort: if no log source is available, the empty string is returned
// and the dashboard simply omits the error excerpt. On Linux it reads the
// systemd journal via journalctl; on macOS it tails ~/Library/Logs/lerd/
// where launchd redirects each unit's stdout+stderr.
func readLastError(unit string) string {
	if line := readLastErrorPlatform(unit); line != "" {
		if len(line) > lastErrorMaxLen {
			line = line[:lastErrorMaxLen] + "…"
		}
		return line
	}
	return ""
}

// enrichBudget caps total time spent reading journals across all units in
// one Enrich call so a slow journal can't stall the snapshot rebuild.
const enrichBudget = 500 * time.Millisecond

// Enrich populates LastError on every entry by reading the journal once per
// unit. Walks in slice order until the per-call budget is hit, leaving any
// remaining entries' LastError empty. Safe with a nil or empty slice.
// Intended for the dashboard pre-serialization step where there are
// typically 0–3 entries, so the budget is rarely exercised.
func Enrich(in []UnhealthyWorker) []UnhealthyWorker {
	deadline := time.Now().Add(enrichBudget)
	for i := range in {
		if in[i].LastError != "" {
			continue
		}
		// A cleanly-stopped worker has no error — its last journal line is just
		// the "Stopped …" / CPU-summary message, which reads as a false error.
		if in[i].State == "expected-but-stopped" {
			continue
		}
		// An unreachable worker's process is still active, so its last journal line
		// is whatever the live server last logged (often a harmless info line) and
		// would read as a false cause. State the real problem instead of the journal.
		// Framework-agnostic on purpose: which file/port carries the URL lives in the
		// store, not here.
		if in[i].State == "unreachable" {
			in[i].LastError = "process is up but its server is not accepting connections"
			continue
		}
		if time.Now().After(deadline) {
			break
		}
		in[i].LastError = lastErrorFn(in[i].Unit)
	}
	return in
}

// Detect returns every worker unit systemd considers "failed". Cheap by
// design: it reads only the existing batched unit-state cache (one
// systemctl call per 3s, shared with the dashboard's enrichment path) plus
// sites.yaml. For a site that has an active health-probed worker it resolves
// the framework once per site, memoised by composer.json mtime, so a steady
// tick reparses nothing. No extra subprocess calls; safe to invoke from a
// hot endpoint.
//
// Two health problems are detected. "failed" — units that hit Restart= rate
// limits or crash repeatedly and stay stuck until reset. "expected-but-stopped"
// — units that are still enabled (systemd's wants-symlink present) yet inactive:
// `lerd worker stop` disables the unit, so an enabled-yet-stopped worker was
// knocked out some other way (an FPM restart cascading through BindsTo, a
// manual `systemctl stop`, a clean exit) and is drift, not intent. Plain
// "inactive" alone is NOT enough — that's why the enabled check matters — and
// timer-driven oneshots (a .timer sibling owns the lifecycle) are normally idle
// between ticks, so they're left alone to avoid false positives.
func Detect() ([]UnhealthyWorker, error) {
	// When lerd was intentionally stopped, its workers are meant to be down, so
	// reporting them as failing/expected-but-stopped is noise: the fix is `lerd
	// start`, not a heal. Suppress detection (and the notifications, banners, and
	// heals that read it) until the next start clears the marker.
	if isStoppedFn() {
		return nil, nil
	}
	reg, err := config.LoadSites()
	if err != nil {
		return nil, err
	}
	// Every active site's checkout, so a unit's WorkingDirectory can be told apart
	// from a worktree's.
	sitePaths := make(map[string]string, len(reg.Sites))
	// path + framework per site, for resolving a health-probed worker's block.
	type siteMeta struct{ path, framework string }
	meta := make(map[string]siteMeta, len(reg.Sites))
	// Workers a site has intentionally idle-suspended must never be reported as
	// failing or drifted — they are asleep on purpose and resume on the next
	// request, so flagging or healing them would be noise (and a heal would wake
	// the site). Index them so detection can skip them.
	suspended := make(map[string]map[string]bool, len(reg.Sites))
	for _, s := range reg.Sites {
		if s.Paused || s.Ignored {
			continue
		}
		sitePaths[s.Name] = s.Path
		meta[s.Name] = siteMeta{path: s.Path, framework: s.Framework}
		if len(s.IdleSuspendedWorkers) > 0 {
			set := make(map[string]bool, len(s.IdleSuspendedWorkers))
			for _, w := range s.IdleSuspendedWorkers {
				set[w] = true
			}
			suspended[s.Name] = set
		}
	}
	if len(sitePaths) == 0 {
		return nil, nil
	}

	states := unitStatesFn()
	// Per-unit ActiveEnter + WorkingDirectory from the same batched snapshot,
	// used to resolve worktree units (by WorkingDirectory) and to gate the dial
	// (by ActiveEnter). Empty on darwin; callers fall back to the prior behaviour.
	unitMeta := unitMetaFn()
	// Framework resolved lazily, at most once per site with an active worker.
	// GetFrameworkForDir is not a plain lookup (it can trigger an unthrottled
	// store fetch), so it must never run per worker inside the loop.
	resolvedFw := make(map[string]*config.Framework)
	resolvedSet := make(map[string]bool)
	var out []UnhealthyWorker
	for unit, state := range states {
		// The unit-state cache aliases each .service unit under both
		// "lerd-foo" and "lerd-foo.service"; pick one canonical form so
		// we don't emit duplicates. .timer units (paired oneshot
		// schedulers) are skipped — their .service sibling, if it ever
		// fails, will surface here under its own key.
		if !strings.HasSuffix(unit, ".service") {
			continue
		}
		// Only these states can be unhealthy; skip the rest before the
		// site-name resolution below so the hot path stays cheap.
		if state != "failed" && state != "inactive" && state != "active" {
			continue
		}
		body := strings.TrimPrefix(unit, "lerd-")
		body = strings.TrimSuffix(body, ".service")
		// Resolve site + worker (longest site suffix for parents; the unit's
		// WorkingDirectory disambiguates worktree units). probePath is the
		// worktree checkout for a worktree unit, else "" (use the site path).
		site, worker, probePath := resolveWorkerUnit(body, sitePaths, unitMeta[unit].WorkingDir)
		if site == "" || worker == "" {
			continue
		}
		if nonWorkerPerSitePrefixes[worker] {
			continue
		}
		if suspended[site][worker] {
			continue // intentionally idle-suspended, not a failure
		}
		var detected string
		switch state {
		case "failed":
			detected = "failed"
		case "inactive":
			// A timer-driven oneshot is meant to sit idle between ticks; its
			// .timer sibling owns the lifecycle, so don't flag the service.
			if _, hasTimer := states[strings.TrimSuffix(unit, ".service")+".timer"]; hasTimer {
				continue
			}
			// Only flag drift, not an intentionally-stopped (disabled) worker.
			if !unitEnabledFn(unit) {
				continue
			}
			detected = "expected-but-stopped"
		default: // "active": up, but a health-probed server may have died under it.
			m := meta[site]
			if !resolvedSet[site] {
				resolvedFw[site], _ = config.GetFrameworkForDir(m.framework, m.path)
				resolvedSet[site] = true
			}
			path := probePath // worktree checkout, or the site root for a parent
			if path == "" {
				path = m.path
			}
			reachable, probed := workerReachableFn(path, resolvedFw[site], worker, unitMeta[unit].ActiveEnter)
			if !probed || reachable {
				continue // no health probe declared, or the server is serving
			}
			detected = "unreachable"
		}
		out = append(out, UnhealthyWorker{
			Site:   site,
			Worker: worker,
			Unit:   strings.TrimSuffix(unit, ".service"),
			State:  detected,
		})
	}
	return out, nil
}

// HealUnit clears any failed state and starts the named worker unit. The
// single "fix this" primitive — every surface (CLI / UI / TUI / MCP) goes
// through here. Crucially, it does NOT touch .lerd.yaml or rewrite the
// unit file: a failed worker is a transient runtime condition, not a
// change of user intent. The reset-failed step is implicit: on Linux,
// systemd.DBusStartUnit calls DBusResetFailed first; on macOS launchd's
// bootstrap path replaces the job entirely.
func HealUnit(unit string) error {
	return healFn(unit)
}

// HealAll detects every unhealthy worker and heals them in order. emit,
// when non-nil, receives one Event per phase transition so the dashboard's
// banner and the MCP tool can stream progress instead of blocking on a
// final summary.
func HealAll(emit func(Event)) (Result, error) {
	if emit == nil {
		emit = func(Event) {}
	}
	unhealthy, err := Detect()
	if err != nil {
		emit(Event{Phase: "failed", Error: err.Error()})
		return Result{}, err
	}
	report := Result{}
	for _, u := range unhealthy {
		emit(Event{Phase: "starting", Site: u.Site, Unit: u.Unit})
		// An unreachable worker's process is still up, so a plain start is a no-op;
		// restart to rebind its server. Failed/stopped workers just start.
		heal := HealUnit
		if u.State == "unreachable" {
			heal = restartFn
		}
		if err := heal(u.Unit); err != nil {
			report.Failed = append(report.Failed, Failure{Worker: u, Err: err.Error()})
			emit(Event{Phase: "failed", Site: u.Site, Unit: u.Unit, Error: err.Error()})
			continue
		}
		report.Healed = append(report.Healed, u)
		emit(Event{Phase: "healed", Site: u.Site, Unit: u.Unit})
	}
	emit(Event{Phase: "done"})
	return report, nil
}

// Summary renders a one-line CLI-friendly summary of a Result.
func Summary(r Result) string {
	if len(r.Healed) == 0 && len(r.Failed) == 0 {
		return "No unhealthy workers."
	}
	if len(r.Failed) == 0 {
		return fmt.Sprintf("Healed %d worker(s).", len(r.Healed))
	}
	return fmt.Sprintf("Healed %d worker(s), %d failed.", len(r.Healed), len(r.Failed))
}
