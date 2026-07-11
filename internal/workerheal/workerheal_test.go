package workerheal

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

// stubEnv stages a sites.yaml with the given names in a temp XDG_DATA_HOME
// and swaps out the unit-state and heal hooks so the test never touches
// real systemd or podman.
func stubEnv(t *testing.T, sites []string, paused map[string]bool, states map[string]string, heal func(string) error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	lerdDir := filepath.Join(dir, "lerd")
	if err := os.MkdirAll(lerdDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "sites:\n"
	for _, name := range sites {
		body += "  - name: " + name + "\n"
		body += "    path: /tmp/" + name + "\n"
		body += "    domains: [" + name + ".test]\n"
		if paused[name] {
			body += "    paused: true\n"
		}
	}
	if err := os.WriteFile(filepath.Join(lerdDir, "sites.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write sites.yaml: %v", err)
	}

	prevStates, prevHeal, prevEnabled := unitStatesFn, healFn, unitEnabledFn
	unitStatesFn = func() map[string]string {
		out := make(map[string]string, len(states))
		for k, v := range states {
			out[k] = v
		}
		return out
	}
	healFn = heal
	// Default to "disabled" so existing failed-state tests are unaffected by
	// the expected-but-stopped path; tests that exercise it override this.
	unitEnabledFn = func(string) bool { return false }
	t.Cleanup(func() {
		unitStatesFn = prevStates
		healFn = prevHeal
		unitEnabledFn = prevEnabled
	})
}

func unitNames(ws []UnhealthyWorker) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Unit
	}
	sort.Strings(out)
	return out
}

func TestDetect_FailedWorkerReturned(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp.service": "failed",
			"lerd-php85-fpm.service":   "active",
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if names := unitNames(got); len(names) != 1 || names[0] != "lerd-queue-myapp" {
		t.Errorf("got %v, want [lerd-queue-myapp]", names)
	}
	if got[0].Site != "myapp" || got[0].Worker != "queue" {
		t.Errorf("site/worker split: site=%q worker=%q", got[0].Site, got[0].Worker)
	}
}

func TestDetect_SuppressedWhenStopped(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp.service": "failed",
			"lerd-php85-fpm.service":   "inactive",
		},
		nil,
	)
	prev := isStoppedFn
	isStoppedFn = func() bool { return true }
	t.Cleanup(func() { isStoppedFn = prev })

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no unhealthy workers while lerd is stopped, got %v", unitNames(got))
	}
}

func TestDetect_PausedSiteExcluded(t *testing.T) {
	stubEnv(t,
		[]string{"alpha", "beta"},
		map[string]bool{"beta": true},
		map[string]string{
			"lerd-queue-alpha.service": "failed",
			"lerd-queue-beta.service":  "failed",
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if names := unitNames(got); len(names) != 1 || names[0] != "lerd-queue-alpha" {
		t.Errorf("paused site bled in: got %v", names)
	}
}

func TestDetect_NonWorkerPerSiteUnitsSkipped(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-fp-myapp.service":     "failed", // per-site FrankenPHP container, not a worker
			"lerd-custom-myapp.service": "failed", // per-site custom container, not a worker
			"lerd-queue-myapp.service":  "failed", // worker
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if names := unitNames(got); len(names) != 1 || names[0] != "lerd-queue-myapp" {
		t.Errorf("non-worker per-site units leaked into detection: %v", names)
	}
}

func TestDetect_GlobalUnitsSkipped(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-php85-fpm.service": "failed", // global FPM unit
			"lerd-nginx.service":     "failed",
			"lerd-dns.service":       "failed",
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("global units flagged as worker drift: %v", unitNames(got))
	}
}

func TestDetect_HyphenatedWorkerName(t *testing.T) {
	// Custom worker with a hyphen in its name plus a site whose name itself
	// contains hyphens — make sure the longest-suffix match keeps both.
	stubEnv(t,
		[]string{"tallyboard"}, nil,
		map[string]string{
			"lerd-emit-events-tallyboard.service": "failed",
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Worker != "emit-events" || got[0].Site != "tallyboard" {
		t.Errorf("split wrong: %+v", got)
	}
}

func TestDetect_DeduplicatesAliasedCacheEntries(t *testing.T) {
	// The siteinfo unit-state cache aliases every .service unit under both
	// "lerd-foo" and "lerd-foo.service". Detect must emit each unit only once.
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp":         "failed",
			"lerd-queue-myapp.service": "failed",
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d entries, want 1: %+v", len(got), got)
	}
}

func TestDetect_OnlyFailedStateMatches(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp.service":    "active",
			"lerd-schedule-myapp.service": "inactive",
			"lerd-reverb-myapp.service":   "failed",
		},
		nil,
	)

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if names := unitNames(got); len(names) != 1 || names[0] != "lerd-reverb-myapp" {
		t.Errorf("got %v, want [lerd-reverb-myapp]", names)
	}
}

func TestDetect_EnabledStoppedWorkerFlagged(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp.service": "inactive",
			"lerd-php85-fpm.service":   "active",
		},
		nil,
	)
	// Enabled yet inactive = drift (e.g. an FPM restart cascaded through BindsTo).
	unitEnabledFn = func(u string) bool { return u == "lerd-queue-myapp.service" }

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Unit != "lerd-queue-myapp" {
		t.Fatalf("got %+v, want one lerd-queue-myapp", got)
	}
	if got[0].State != "expected-but-stopped" {
		t.Errorf("state = %q, want expected-but-stopped", got[0].State)
	}
}

func TestDetect_DisabledStoppedWorkerIgnored(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{"lerd-queue-myapp.service": "inactive"},
		nil,
	)
	// stubEnv defaults enabled=false: a disabled stopped worker was stopped on
	// purpose (`lerd worker stop` disables), so it must not be flagged.
	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("disabled stopped worker should be ignored, got %+v", got)
	}
}

func TestDetect_TimerDrivenServiceNotFlagged(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-schedule-myapp.service": "inactive",
			"lerd-schedule-myapp.timer":   "active",
		},
		nil,
	)
	// Even enabled, a timer-driven oneshot is normally idle between ticks.
	unitEnabledFn = func(string) bool { return true }

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("timer-driven idle oneshot must not be flagged, got %+v", got)
	}
}

func TestHealAll_EmitsEventsAndReports(t *testing.T) {
	healed := []string{}
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp.service":  "failed",
			"lerd-reverb-myapp.service": "failed",
		},
		func(unit string) error {
			healed = append(healed, unit)
			return nil
		},
	)

	var events []Event
	report, err := HealAll(func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("HealAll: %v", err)
	}

	if len(report.Healed) != 2 || len(report.Failed) != 0 {
		t.Errorf("report: %+v", report)
	}
	sort.Strings(healed)
	if len(healed) != 2 || healed[0] != "lerd-queue-myapp" || healed[1] != "lerd-reverb-myapp" {
		t.Errorf("heal calls: %v", healed)
	}
	// Each worker should produce at least starting + healed; the loop ends
	// with a single done event.
	if events[len(events)-1].Phase != "done" {
		t.Errorf("missing done event; got %+v", events)
	}
	var startCount, healCount int
	for _, e := range events {
		switch e.Phase {
		case "starting":
			startCount++
		case "healed":
			healCount++
		}
	}
	if startCount != 2 || healCount != 2 {
		t.Errorf("events: %d starting, %d healed (want 2 each)", startCount, healCount)
	}
}

func TestHealAll_CapturesPerUnitFailures(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-queue-myapp.service":  "failed",
			"lerd-reverb-myapp.service": "failed",
		},
		func(unit string) error {
			if unit == "lerd-reverb-myapp" {
				return errors.New("boom")
			}
			return nil
		},
	)

	report, err := HealAll(nil)
	if err != nil {
		t.Fatalf("HealAll: %v", err)
	}
	if len(report.Healed) != 1 || report.Healed[0].Unit != "lerd-queue-myapp" {
		t.Errorf("healed: %+v", report.Healed)
	}
	if len(report.Failed) != 1 || report.Failed[0].Worker.Unit != "lerd-reverb-myapp" {
		t.Errorf("failed: %+v", report.Failed)
	}
	if got := Summary(report); got != "Healed 1 worker(s), 1 failed." {
		t.Errorf("summary = %q", got)
	}
}

func TestSummary_NoUnhealthy(t *testing.T) {
	if got := Summary(Result{}); got != "No unhealthy workers." {
		t.Errorf("Summary(empty) = %q", got)
	}
}

func TestEnrich_PopulatesLastError(t *testing.T) {
	prev := lastErrorFn
	t.Cleanup(func() { lastErrorFn = prev })
	lastErrorFn = func(unit string) string { return "boom on " + unit }

	in := []UnhealthyWorker{
		{Unit: "lerd-queue-foo", Site: "foo", Worker: "queue", State: "failed"},
		{Unit: "lerd-schedule-bar", Site: "bar", Worker: "schedule", State: "failed"},
	}
	out := Enrich(in)
	if out[0].LastError != "boom on lerd-queue-foo" {
		t.Errorf("queue last_error = %q", out[0].LastError)
	}
	if out[1].LastError != "boom on lerd-schedule-bar" {
		t.Errorf("schedule last_error = %q", out[1].LastError)
	}
}

func TestEnrich_KeepsPreSetErrorAndSkipsCall(t *testing.T) {
	prev := lastErrorFn
	t.Cleanup(func() { lastErrorFn = prev })
	calls := 0
	lastErrorFn = func(unit string) string {
		calls++
		return "fresh"
	}

	in := []UnhealthyWorker{
		{Unit: "a", LastError: "preserved"},
		{Unit: "b"},
	}
	out := Enrich(in)
	if out[0].LastError != "preserved" {
		t.Errorf("pre-set error overwritten: %q", out[0].LastError)
	}
	if out[1].LastError != "fresh" {
		t.Errorf("missing fill: %q", out[1].LastError)
	}
	if calls != 1 {
		t.Errorf("lastErrorFn calls = %d, want 1", calls)
	}
}

func TestEnrich_UnreachableGetsDedicatedLineNotJournal(t *testing.T) {
	prev := lastErrorFn
	t.Cleanup(func() { lastErrorFn = prev })
	// The journal line for an active server would read as a false cause; Enrich
	// must not call it for an unreachable worker.
	lastErrorFn = func(string) string {
		t.Fatal("readLastError must not be called for an unreachable worker")
		return ""
	}

	out := Enrich([]UnhealthyWorker{{Unit: "lerd-vite-foo", Site: "foo", Worker: "vite", State: "unreachable"}})
	if out[0].LastError == "" || out[0].LastError == "boom" {
		t.Errorf("unreachable last_error = %q, want the dedicated not-accepting line", out[0].LastError)
	}
}

func TestEnrich_NilAndEmpty(t *testing.T) {
	if got := Enrich(nil); got != nil {
		t.Errorf("Enrich(nil) = %v, want nil", got)
	}
	if got := Enrich([]UnhealthyWorker{}); len(got) != 0 {
		t.Errorf("Enrich(empty) = %v, want empty", got)
	}
}

// An active worker whose declared server has died (process up, port refused) is
// flagged "unreachable"; a plain active worker with no health probe is not.
func TestDetect_UnreachableActiveWorkerFlagged(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{
			"lerd-vite-myapp.service":  "active",
			"lerd-queue-myapp.service": "active",
		},
		nil,
	)
	prev := workerReachableFn
	workerReachableFn = func(_ string, _ *config.Framework, worker string, _ time.Time) (reachable, probed bool) {
		if worker == "vite" {
			return false, true // process up, server not accepting
		}
		return false, false // no health probe for other workers
	}
	t.Cleanup(func() { workerReachableFn = prev })

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Unit != "lerd-vite-myapp" {
		t.Fatalf("got %v, want [lerd-vite-myapp]", unitNames(got))
	}
	if got[0].State != "unreachable" {
		t.Errorf("state = %q, want unreachable", got[0].State)
	}
}

func TestResolveWorkerUnit(t *testing.T) {
	sites := map[string]bool{"ws": true, "feat": true, "app": true}

	if s, w, p := resolveWorkerUnit("vite-app", sites, ""); s != "app" || w != "vite" || p != "" {
		t.Errorf("parent: got %q/%q/%q, want app/vite/empty", s, w, p)
	}
	// WorkingDirectory pins the site to "ws" even though "feat" is a longer
	// candidate that a plain longest-match would pick.
	if s, w, p := resolveWorkerUnit("vite-ws-feat-x", sites, "/home/u/wt/feat-x"); s != "ws" || w != "vite" || p != "/home/u/wt/feat-x" {
		t.Errorf("worktree: got %q/%q/%q, want ws/vite//home/u/wt/feat-x", s, w, p)
	}
	// Without a WorkingDirectory a worktree unit is unresolvable, so it is skipped
	// rather than mis-parsed.
	if s, _, _ := resolveWorkerUnit("vite-ws-feat-x", sites, ""); s != "" {
		t.Errorf("no workingdir: got site %q, want empty", s)
	}
}

func TestDetect_UnreachableWorktreeWorkerFlagged(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{"lerd-vite-myapp-featx.service": "active"},
		nil,
	)
	prevReach, prevMeta := workerReachableFn, unitMetaFn
	workerReachableFn = func(_ string, _ *config.Framework, worker string, _ time.Time) (bool, bool) {
		return false, worker == "vite" // vite: process up, not accepting
	}
	unitMetaFn = func() map[string]siteinfo.UnitMeta {
		return map[string]siteinfo.UnitMeta{
			"lerd-vite-myapp-featx.service": {WorkingDir: "/home/u/wt/featx"},
		}
	}
	t.Cleanup(func() { workerReachableFn = prevReach; unitMetaFn = prevMeta })

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 1 || got[0].Unit != "lerd-vite-myapp-featx" || got[0].State != "unreachable" {
		t.Fatalf("got %v, want [lerd-vite-myapp-featx unreachable]", unitNames(got))
	}
}

func TestDetect_ReachableActiveWorkerNotFlagged(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{"lerd-vite-myapp.service": "active"},
		nil,
	)
	prev := workerReachableFn
	workerReachableFn = func(_ string, _ *config.Framework, _ string, _ time.Time) (bool, bool) { return true, true } // serving
	t.Cleanup(func() { workerReachableFn = prev })

	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a reachable server must not be flagged, got %v", unitNames(got))
	}
}

// An unreachable worker's process is still up, so heal must restart it (a start
// is a no-op on an active unit), while failed/stopped workers still start.
func TestHealAll_RestartsUnreachableWorker(t *testing.T) {
	stubEnv(t,
		[]string{"myapp"}, nil,
		map[string]string{"lerd-vite-myapp.service": "active"},
		func(string) error { t.Fatal("unreachable worker must be restarted, not started"); return nil },
	)
	prevReach, prevRestart := workerReachableFn, restartFn
	workerReachableFn = func(_ string, _ *config.Framework, _ string, _ time.Time) (bool, bool) { return false, true }
	var restarted string
	restartFn = func(unit string) error { restarted = unit; return nil }
	t.Cleanup(func() { workerReachableFn = prevReach; restartFn = prevRestart })

	res, err := HealAll(nil)
	if err != nil {
		t.Fatalf("HealAll: %v", err)
	}
	if restarted != "lerd-vite-myapp" {
		t.Errorf("restarted %q, want lerd-vite-myapp", restarted)
	}
	if len(res.Healed) != 1 {
		t.Errorf("healed %d, want 1", len(res.Healed))
	}
}
