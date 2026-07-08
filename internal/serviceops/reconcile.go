package serviceops

import (
	"errors"
	"fmt"
	"slices"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"gopkg.in/yaml.v3"
)

// Seams so tests can drive reconcile without real podman/quadlet work.
var (
	ensureQuadletFn          = EnsureCustomServiceQuadlet
	listManagedServiceNames  = podman.ListManagedServiceNames
	orphanContainerRunningFn = podman.ContainerRunningQuiet
	materializeFilesFn       = config.MaterializeServiceFiles
	newestFileMtimeFn        = config.ServiceFilesNewestMtime
	containerStartedAtFn     = podman.ContainerStartedAt
	restartUnitFn            = podman.RestartUnit
)

// ReconcileResult reports what ReconcileServices changed.
type ReconcileResult struct {
	QuadletsRegenerated   []string // YAML present, unit was missing and regenerated
	OrphansRemoved        []string // service quadlet with no YAML, removed
	RunningOrphansSkipped []string // orphan left in place because its container is up
	DefinitionsRefreshed  []string // preset-backed YAML re-materialised from the store
	ConfigsApplied        []string // preset config file drifted; rewritten and the running service restarted
}

// ReconcileServices enforces the issue #678 invariant: regenerate a missing
// unit from its YAML, remove an orphan service quadlet (no YAML; data left).
// A running orphan is skipped, not destroyed. Per-item errors are collected so
// one bad service can't block the rest.
func ReconcileServices(emit func(PhaseEvent)) (ReconcileResult, error) {
	if emit == nil {
		emit = func(PhaseEvent) {}
	}
	var res ReconcileResult
	var errs []error

	// Forward: every defined service must have its unit (EnsureCustomServiceQuadlet
	// also re-syncs the macOS plist).
	customs, err := config.ListCustomServices()
	if err != nil {
		return res, fmt.Errorf("listing custom services: %w", err)
	}
	for _, svc := range customs {
		// Re-materialise declarative fields (client_shims, family, dashboard,
		// env mappings…) from the store preset so a store change reaches an
		// already-installed service, preserving install-time pins. EnsurePreset
		// inside also backfills the cache for a service installed by an older
		// lerd. Best-effort: offline or an unresolvable version leaves the
		// snapshot untouched.
		if refreshed, changed := refreshPresetDefinition(svc); changed {
			if err := config.SaveCustomService(refreshed); err != nil {
				errs = append(errs, fmt.Errorf("refreshing %s from preset: %w", svc.Name, err))
			} else {
				svc = refreshed
				res.DefinitionsRefreshed = append(res.DefinitionsRefreshed, svc.Name)
			}
		}
		unitInstalled := UnitInstalledFn("lerd-" + svc.Name)
		if unitInstalled {
			if applied, err := RestartIfConfigDrifted(svc.Name, svc.Preset); err != nil {
				errs = append(errs, err)
			} else if applied {
				res.ConfigsApplied = append(res.ConfigsApplied, svc.Name)
			}
			if !slices.Contains(res.DefinitionsRefreshed, svc.Name) {
				continue
			}
		}
		// Regenerate the quadlet when the unit is missing, or when the definition
		// changed. WriteQuadletDiff is a no-op when the content is identical, so a
		// client_shims-only change (which never appears in the quadlet) is free.
		if err := ensureQuadletFn(svc); err != nil {
			errs = append(errs, fmt.Errorf("regenerating quadlet for %s: %w", svc.Name, err))
			continue
		}
		if !unitInstalled {
			res.QuadletsRegenerated = append(res.QuadletsRegenerated, svc.Name)
		}
	}

	// Backward: a service quadlet with no YAML is an orphan. Candidates are marked
	// quadlets (any name, covers user-defined services) plus known non-default
	// preset names (covers legacy orphans written before the marker existed).
	for _, name := range orphanCandidates() {
		if config.IsDefaultPreset(name) || config.CustomServiceExists(name) {
			continue
		}
		if !podman.QuadletInstalled("lerd-" + name) {
			continue
		}
		// Don't destroy a workload that's still up; leave it for explicit removal.
		if orphanContainerRunningFn("lerd-" + name) {
			res.RunningOrphansSkipped = append(res.RunningOrphansSkipped, name)
			continue
		}
		if err := RemoveService(name, RemoveOptions{}, emit); err != nil {
			errs = append(errs, fmt.Errorf("removing orphan service %s: %w", name, err))
			continue
		}
		res.OrphansRemoved = append(res.OrphansRemoved, name)
	}
	return res, errors.Join(errs...)
}

// RestartIfConfigDrifted re-materialises a running service's preset config files
// and, when the config file is newer than the container's boot (i.e. the container
// is on stale config after a shipped preset change like a higher max_allowed_packet),
// restarts it, returning whether it did. It is the single seam both the custom
// reconcile and the default-stack start pass use, since those flow through separate
// install paths. A stopped or missing container, or a preset with no config files,
// is a no-op; the mtime only advances on a real content change, so a steady state
// never restarts anything.
func RestartIfConfigDrifted(name, preset string) (bool, error) {
	started, running := containerStartedAtFn("lerd-" + name)
	if !running {
		return false, nil
	}
	probe := &config.CustomService{Name: name, Preset: preset}
	if err := materializeFilesFn(probe); err != nil {
		return false, fmt.Errorf("materialising files for %s: %w", name, err)
	}
	mtime, ok := newestFileMtimeFn(probe)
	if !ok || !mtime.After(started) {
		return false, nil
	}
	if err := restartUnitFn("lerd-" + name); err != nil {
		return false, fmt.Errorf("restarting %s after a config change: %w", name, err)
	}
	return true, nil
}

// refreshPresetDefinition re-resolves a preset-backed service from the store
// preset so declarative additions (client_shims, family, dashboard…) reach an
// already-installed service, while preserving install-time pins: the running
// image, rollback/migrate state, and identity. Returns the refreshed definition
// and whether it changed. A non-preset service, an unresolvable preset, or a
// removed version leaves the snapshot unchanged.
func refreshPresetDefinition(svc *config.CustomService) (*config.CustomService, bool) {
	if svc.Preset == "" {
		return svc, false
	}
	preset, err := config.EnsurePreset(svc.Preset)
	if err != nil {
		return svc, false
	}
	// A canonical install carries the bare preset name; resolve it pinned so a
	// later canonical-version flip in the store never renames it (which would
	// rewrite its {{name}}-templated DB_HOST/connection_url to a container that
	// does not exist). An alternate install keeps its version-suffixed name.
	var fresh *config.CustomService
	// ResolvePinned pins the canonical instance to its version's port, but only a
	// multi-version preset has versions to pin; a single-version preset (no
	// versions list) must go through Resolve or it never receives store refreshes.
	if svc.Name == preset.Name && len(preset.Versions) > 0 {
		fresh, err = preset.ResolvePinned(svc.PresetVersion)
	} else {
		fresh, err = preset.Resolve(svc.PresetVersion)
	}
	if err != nil {
		return svc, false
	}
	// Preserve install-time pins and state; everything else comes from the
	// store. Image stays put so a passive reconcile never upgrades the running
	// container, that remains an explicit `lerd service update`/`migrate`.
	fresh.Image = svc.Image
	fresh.PreviousImage = svc.PreviousImage
	fresh.LastOp = svc.LastOp
	fresh.PreMigrateBackup = svc.PreMigrateBackup
	fresh.Name = svc.Name
	fresh.Preset = svc.Preset
	fresh.PresetVersion = svc.PresetVersion
	// Ports and the connection URL carry the host-port allocation chosen at
	// quadlet generation (shifted when the preset default collided), not store
	// intent, so preserve them like the image. Otherwise reconcile resets a
	// shifted service to the default port on every start, churning the snapshot
	// and re-running shim reconciliation for no real change.
	if len(svc.Ports) > 0 {
		fresh.Ports = svc.Ports
	}
	if svc.ConnectionURL != "" {
		fresh.ConnectionURL = svc.ConnectionURL
	}

	if sameDefinition(fresh, svc) {
		return svc, false
	}
	return fresh, true
}

// sameDefinition compares two service definitions by their serialised form.
func sameDefinition(a, b *config.CustomService) bool {
	ay, err1 := yaml.Marshal(a)
	by, err2 := yaml.Marshal(b)
	return err1 == nil && err2 == nil && string(ay) == string(by)
}

// orphanCandidates is the deduped set of service names worth checking for an
// orphan quadlet: marked quadlets present on disk, plus every non-default preset
// name (so legacy pre-marker orphans are still reaped). Preset names never
// collide with site/worker unit names, so this stays safe.
func orphanCandidates() []string {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, n := range listManagedServiceNames() {
		add(n)
	}
	presets, err := config.ListPresets()
	if err == nil {
		for _, p := range presets {
			if config.IsDefaultPreset(p.Name) {
				continue
			}
			add(p.Name)
			for _, v := range p.Versions {
				add(config.PresetVersionServiceName(p.Name, v))
			}
		}
	}
	return names
}
