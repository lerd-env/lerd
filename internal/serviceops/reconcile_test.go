package serviceops

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// reconcileEnv sets up an isolated HOME/XDG tree and a no-op daemon reload so
// ReconcileServices can write/read quadlets without touching the real host.
func reconcileEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }
}

// writeQuadlet writes a quadlet for name; marked=true tags it as a managed
// service (what GenerateCustomQuadlet does), marked=false mimics a site/worker
// quadlet that reconcile must never touch.
func writeQuadlet(t *testing.T, name string, marked bool) {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	body := "[Container]\nImage=docker.io/example/" + name + ":latest\n"
	if marked {
		body = podman.CustomServiceQuadletMarker + "\n" + body
	}
	if err := os.WriteFile(filepath.Join(dir, "lerd-"+name+".container"), []byte(body), 0o644); err != nil {
		t.Fatalf("write quadlet %s: %v", name, err)
	}
}

// refreshPresetDefinition must refresh a preset service's declarative fields from
// the store while preserving install-time pins. A regression in the preservation
// list would silently overwrite a running service's pinned image on reconcile.
func TestRefreshPresetDefinition_RefreshesDeclarativePreservesPins(t *testing.T) {
	reconcileEnv(t)

	presetDir := config.StorePresetsDir()
	if err := os.MkdirAll(presetDir, 0o755); err != nil {
		t.Fatalf("mkdir preset dir: %v", err)
	}
	// The store now describes a newer definition: fresh description and dashboard.
	presetYAML := "name: myadmin\n" +
		"description: new description\n" +
		"dashboard: http://localhost:9000\n" +
		"versions:\n" +
		"  - tag: \"1\"\n" +
		"    image: myimg:2\n" +
		"    canonical: true\n"
	if err := os.WriteFile(filepath.Join(presetDir, "myadmin.yaml"), []byte(presetYAML), 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}

	installed := &config.CustomService{
		Name:          "myadmin",
		Preset:        "myadmin",
		PresetVersion: "1",
		Image:         "myimg:1", // pinned: the running image, must not be upgraded
		LastOp:        "install", // pinned op state
		Description:   "old description",
	}

	fresh, changed := refreshPresetDefinition(installed)
	if !changed {
		t.Fatal("a differing store definition should report changed")
	}
	if fresh.Description != "new description" || fresh.Dashboard != "http://localhost:9000" {
		t.Errorf("declarative fields not refreshed from the store: %+v", fresh)
	}
	if fresh.Image != "myimg:1" {
		t.Errorf("pinned image must be preserved, got %q", fresh.Image)
	}
	if fresh.LastOp != "install" {
		t.Errorf("pinned op state must be preserved, got %q", fresh.LastOp)
	}

	// An identical store definition must be a no-op, so a steady-state reconcile
	// never rewrites the service snapshot.
	installed.Description = "new description"
	installed.Dashboard = "http://localhost:9000"
	if _, changed := refreshPresetDefinition(installed); changed {
		t.Error("an identical store definition should not report changed")
	}
}

// A single-version preset (no versions list) installs under its bare canonical
// name, so refreshPresetDefinition must resolve it too; routing it through
// ResolvePinned would error and silently skip every store refresh for it.
func TestRefreshPresetDefinition_SingleVersionPresetRefreshes(t *testing.T) {
	reconcileEnv(t)

	presetDir := config.StorePresetsDir()
	if err := os.MkdirAll(presetDir, 0o755); err != nil {
		t.Fatalf("mkdir preset dir: %v", err)
	}
	presetYAML := "name: pdfsvc\n" +
		"image: docker.io/gotenberg/gotenberg:8\n" +
		"description: PDF service\n" +
		"dashboard: http://localhost:3000\n"
	if err := os.WriteFile(filepath.Join(presetDir, "pdfsvc.yaml"), []byte(presetYAML), 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}

	installed := &config.CustomService{
		Name:   "pdfsvc",
		Preset: "pdfsvc",
		Image:  "docker.io/gotenberg/gotenberg:8",
		LastOp: "install",
		// dashboard missing: a store addition that reconcile must now deliver.
	}
	fresh, changed := refreshPresetDefinition(installed)
	if !changed {
		t.Fatal("single-version preset should refresh from the store, got changed=false")
	}
	if fresh.Dashboard != "http://localhost:3000" || fresh.Description != "PDF service" {
		t.Errorf("declarative fields not refreshed: %+v", fresh)
	}
}

// A service whose host port was shifted off the preset default (a collision at
// quadlet generation) must not be seen as changed on every reconcile, or the
// snapshot churns and shim reconciliation reruns each start.
func TestRefreshPresetDefinition_PreservesShiftedPortNoChurn(t *testing.T) {
	reconcileEnv(t)

	presetDir := config.StorePresetsDir()
	if err := os.MkdirAll(presetDir, 0o755); err != nil {
		t.Fatalf("mkdir preset dir: %v", err)
	}
	presetYAML := "name: mydb\n" +
		"description: db\n" +
		"ports:\n" +
		"  - \"{{host_port}}:3306\"\n" +
		"connection_url: mysql://root:lerd@127.0.0.1:{{host_port}}/lerd\n" +
		"versions:\n" +
		"  - tag: \"11.8\"\n" +
		"    image: docker.io/library/mariadb:11.8\n" +
		"    canonical: true\n" +
		"    host_port: 3306\n"
	if err := os.WriteFile(filepath.Join(presetDir, "mydb.yaml"), []byte(presetYAML), 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}

	installed := &config.CustomService{
		Name:          "mydb",
		Preset:        "mydb",
		PresetVersion: "11.8",
		Image:         "docker.io/library/mariadb:11.8",
		Description:   "db",
		Ports:         []string{"3399:3306"}, // shifted host port
		ConnectionURL: "mysql://root:lerd@127.0.0.1:3399/lerd",
	}
	fresh, changed := refreshPresetDefinition(installed)
	if changed {
		t.Errorf("a store-identical, port-shifted service should not churn; got changed=true (fresh=%+v)", fresh)
	}

	// The shift is preserved (not reset to the preset default), and a real
	// declarative change is still detected.
	installed.Description = "stale"
	fresh, changed = refreshPresetDefinition(installed)
	if !changed {
		t.Fatal("a genuine declarative change should still report changed")
	}
	if len(fresh.Ports) == 0 || fresh.Ports[0] != "3399:3306" {
		t.Errorf("shifted host port not preserved through a refresh: %v", fresh.Ports)
	}
	if fresh.ConnectionURL != "mysql://root:lerd@127.0.0.1:3399/lerd" {
		t.Errorf("shifted connection URL not preserved: %q", fresh.ConnectionURL)
	}
}

// TestReconcileServices_forwardHealsMissingQuadlet: a service whose YAML exists
// but whose quadlet is missing gets its quadlet regenerated (issue #678).
func TestReconcileServices_forwardHealsMissingQuadlet(t *testing.T) {
	reconcileEnv(t)

	if err := config.SaveCustomService(&config.CustomService{Name: "gotenberg", Image: "docker.io/gotenberg/gotenberg:8"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if podman.QuadletInstalled("lerd-gotenberg") {
		t.Fatalf("precondition: quadlet should not exist yet")
	}

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !slices.Contains(res.QuadletsRegenerated, "gotenberg") {
		t.Fatalf("expected gotenberg in QuadletsRegenerated, got %+v", res)
	}
	if !podman.QuadletInstalled("lerd-gotenberg") {
		t.Fatalf("expected quadlet regenerated from the YAML")
	}
}

// TestReconcileServices_removesMarkedOrphans: a marked service quadlet with no
// backing YAML is an orphan and is removed, whether or not its name is a preset
// (covers the user-named orphan gap, issue #678 #4).
func TestReconcileServices_removesMarkedOrphans(t *testing.T) {
	reconcileEnv(t)
	rec := stubPodmanRemove(t)
	rec.currentStatus = "inactive"

	writeQuadlet(t, "gotenberg", true)   // preset-named orphan
	writeQuadlet(t, "acme-widget", true) // user-named orphan

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, want := range []string{"gotenberg", "acme-widget"} {
		if !slices.Contains(res.OrphansRemoved, want) {
			t.Fatalf("expected %s in OrphansRemoved, got %+v", want, res)
		}
		if !slices.Contains(rec.removedQuadlets, "lerd-"+want) {
			t.Fatalf("expected RemoveService to drop lerd-%s, recorder: %+v", want, rec.removedQuadlets)
		}
	}
}

// TestReconcileServices_removesLegacyUnmarkedPresetOrphan: an orphan written by
// an older binary has no marker, but a preset-named quadlet with no YAML is
// still reaped via the preset-name fallback (the existing-drift migration case).
func TestReconcileServices_removesLegacyUnmarkedPresetOrphan(t *testing.T) {
	reconcileEnv(t)
	rec := stubPodmanRemove(t)
	rec.currentStatus = "inactive"

	writeQuadlet(t, "gotenberg", false) // legacy orphan, no marker

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !slices.Contains(res.OrphansRemoved, "gotenberg") {
		t.Fatalf("expected legacy unmarked preset orphan reaped, got %+v", res)
	}
}

// TestReconcileServices_sparesDefaultPreset: built-in default presets carry the
// marker but have no YAML by design, so they must not be treated as orphans.
func TestReconcileServices_sparesDefaultPreset(t *testing.T) {
	reconcileEnv(t)
	rec := stubPodmanRemove(t)

	if !config.IsDefaultPreset("mysql") {
		t.Fatalf("test premise broken: mysql must be a default preset")
	}
	writeQuadlet(t, "mysql", true)

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if slices.Contains(res.OrphansRemoved, "mysql") || slices.Contains(rec.removedQuadlets, "lerd-mysql") {
		t.Fatalf("default preset must never be reaped, res=%+v rec=%+v", res, rec.removedQuadlets)
	}
}

// TestReconcileServices_sparesUnmarkedQuadlet: a quadlet without the managed
// marker (e.g. a site container lerd-custom-<site>) must be left untouched.
func TestReconcileServices_sparesUnmarkedQuadlet(t *testing.T) {
	reconcileEnv(t)
	rec := stubPodmanRemove(t)

	writeQuadlet(t, "custom-gonitro", false) // site custom container, unmarked

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.OrphansRemoved) != 0 || len(rec.removedQuadlets) != 0 {
		t.Fatalf("unmarked quadlet must not be removed, res=%+v rec=%+v", res, rec.removedQuadlets)
	}
}

// TestReconcileServices_skipsRunningOrphan: an orphan whose container is still
// up must be left in place, not force-stopped and destroyed (issue #678 #2).
func TestReconcileServices_skipsRunningOrphan(t *testing.T) {
	reconcileEnv(t)
	rec := stubPodmanRemove(t)

	prev := orphanContainerRunningFn
	t.Cleanup(func() { orphanContainerRunningFn = prev })
	orphanContainerRunningFn = func(string) bool { return true }

	writeQuadlet(t, "gotenberg", true)

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !slices.Contains(res.RunningOrphansSkipped, "gotenberg") {
		t.Fatalf("expected gotenberg in RunningOrphansSkipped, got %+v", res)
	}
	if len(res.OrphansRemoved) != 0 || len(rec.removedQuadlets) != 0 {
		t.Fatalf("running orphan must not be removed, res=%+v rec=%+v", res, rec.removedQuadlets)
	}
}

// TestReconcileServices_continuesPastForwardError: one service that fails to
// regenerate must not block the other heals or the orphan-cleanup pass (#1).
func TestReconcileServices_continuesPastForwardError(t *testing.T) {
	reconcileEnv(t)
	rec := stubPodmanRemove(t)
	rec.currentStatus = "inactive"

	for _, n := range []string{"bad", "good"} {
		if err := config.SaveCustomService(&config.CustomService{Name: n, Image: "docker.io/example/" + n}); err != nil {
			t.Fatalf("save %s: %v", n, err)
		}
	}
	writeQuadlet(t, "gotenberg", true) // orphan that must still be reaped despite the error

	prev := ensureQuadletFn
	t.Cleanup(func() { ensureQuadletFn = prev })
	ensureQuadletFn = func(svc *config.CustomService) error {
		if svc.Name == "bad" {
			return errors.New("boom")
		}
		return prev(svc)
	}

	res, err := ReconcileServices(nil)
	if err == nil || !contains(err.Error(), "bad") {
		t.Fatalf("expected aggregated error mentioning bad, got: %v", err)
	}
	if !slices.Contains(res.QuadletsRegenerated, "good") {
		t.Fatalf("good service must still be regenerated despite bad failing, got %+v", res)
	}
	if !slices.Contains(res.OrphansRemoved, "gotenberg") {
		t.Fatalf("orphan cleanup must still run despite a forward error, got %+v", res)
	}
}

// TestReconcileServices_steadyStateNoop: a fully-installed service (YAML +
// marked quadlet) triggers neither a regeneration nor a removal.
// A shipped preset config-file change (e.g. a higher max_allowed_packet) must
// reach an already-installed, running service on reconcile: when the config file
// is newer than the container's boot, the service is restarted, so the fix lands
// on `lerd update` rather than only on an explicit reinstall.
func TestReconcileServices_appliesDriftedConfigAndRestarts(t *testing.T) {
	reconcileEnv(t)
	if err := config.SaveCustomService(&config.CustomService{Name: "mysql", Image: "docker.io/library/mysql:8.4"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	writeQuadlet(t, "mysql", true)

	boot := time.Unix(1_000_000, 0)
	restore := swapDriftSeams(t,
		func(*config.CustomService) error { return nil },
		func(*config.CustomService) (time.Time, bool) { return boot.Add(time.Hour), true }, // file newer than boot
		func(string) (time.Time, bool) { return boot, true },
		func(unit string) error { return nil },
		func(string) bool { return true },
	)
	defer restore()

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !slices.Contains(res.ConfigsApplied, "mysql") {
		t.Fatalf("expected mysql in ConfigsApplied, got %+v", res)
	}
}

func TestReconcileServices_noRestartWhenConfigCurrent(t *testing.T) {
	reconcileEnv(t)
	if err := config.SaveCustomService(&config.CustomService{Name: "mysql", Image: "docker.io/library/mysql:8.4"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	writeQuadlet(t, "mysql", true)

	boot := time.Unix(1_000_000, 0)
	restarted := false
	restore := swapDriftSeams(t,
		func(*config.CustomService) error { return nil },
		func(*config.CustomService) (time.Time, bool) { return boot.Add(-time.Hour), true }, // file older than boot
		func(string) (time.Time, bool) { return boot, true },
		func(unit string) error { restarted = true; return nil },
		func(string) bool { return true },
	)
	defer restore()

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if restarted {
		t.Fatal("a service booted after its config was written must not be restarted")
	}
	if len(res.ConfigsApplied) != 0 {
		t.Fatalf("expected no ConfigsApplied, got %+v", res)
	}
}

// swapDriftSeams overrides the reconcile config-drift seams for a test, restoring
// them on cleanup.
func swapDriftSeams(t *testing.T, mat func(*config.CustomService) error, mtime func(*config.CustomService) (time.Time, bool), started func(string) (time.Time, bool), restart func(string) error, installed func(string) bool) func() {
	t.Helper()
	pm, pmt, ps, pr, pi := materializeFilesFn, newestFileMtimeFn, containerStartedAtFn, restartUnitFn, UnitInstalledFn
	materializeFilesFn, newestFileMtimeFn, containerStartedAtFn, restartUnitFn, UnitInstalledFn = mat, mtime, started, restart, installed
	return func() {
		materializeFilesFn, newestFileMtimeFn, containerStartedAtFn, restartUnitFn, UnitInstalledFn = pm, pmt, ps, pr, pi
	}
}

func TestReconcileServices_steadyStateNoop(t *testing.T) {
	reconcileEnv(t)

	if err := config.SaveCustomService(&config.CustomService{Name: "gotenberg", Image: "docker.io/gotenberg/gotenberg:8"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	writeQuadlet(t, "gotenberg", true)

	res, err := ReconcileServices(nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.QuadletsRegenerated) != 0 || len(res.OrphansRemoved) != 0 {
		t.Fatalf("steady state must be a no-op, got %+v", res)
	}
}
