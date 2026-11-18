package serviceops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"gopkg.in/yaml.v3"
)

// Seams so tests can drive reconcile without real podman/quadlet work.
var (
	ensureQuadletFn          = ensureCustomServiceQuadletDiff
	listManagedServiceNames  = podman.ListManagedServiceNames
	orphanContainerRunningFn = podman.ContainerRunningQuiet
	materializeFilesFn       = config.MaterializeServiceFiles
	newestFileMtimeFn        = config.ServiceFilesNewestMtime
	containerStartedAtFn     = podman.ContainerStartedAt
	restartUnitFn            = podman.RestartUnit
	quadletMtimeFn           = quadletMtime
)

// quadletMtime reports when a service's unit file was last written, and whether
// it exists at all.
func quadletMtime(name string) (time.Time, bool) {
	info, err := os.Stat(filepath.Join(config.QuadletDir(), "lerd-"+name+".container"))
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// containerOlderThanUnit reports whether a running container was started before
// the unit file it runs from was last written. A unit rewritten by an earlier
// pass — the install refreshes every service quadlet long before it reaches the
// reconcile — leaves nothing for this pass to change, so asking only "did I just
// rewrite it" let a container keep serving a definition that had already moved
// on. Comparing against the container's own start time catches both.
func containerOlderThanUnit(name string) bool {
	started, running := containerStartedAtFn("lerd-" + name)
	if !running {
		return false
	}
	mtime, ok := quadletMtimeFn(name)
	return ok && mtime.After(started)
}

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
		// Render the quadlet every pass and let the content decide. What a unit
		// renders to is not all in the YAML: file mounts and the proxy env come
		// from the preset when the quadlet is written and are never stored on the
		// service, so gating this on the definition having changed left a preset
		// that added either of them unable to reach the unit at all. WriteQuadletDiff
		// is a no-op when the content is identical, so a steady state costs a
		// comparison and a change nobody can see in the YAML still lands.
		//
		// Before the drift restart below, not after: a change that adds a file
		// mount changes the unit too, and restarting the old one brings the
		// container back without the mount, leaving it a pass behind until
		// something restarts it again.
		changed, err := ensureQuadletFn(svc)
		if err != nil {
			errs = append(errs, fmt.Errorf("regenerating quadlet for %s: %w", svc.Name, err))
			continue
		}
		unitChanged := changed
		if !unitInstalled {
			res.QuadletsRegenerated = append(res.QuadletsRegenerated, svc.Name)
		}
		if unitInstalled {
			applied, err := RestartIfConfigDrifted(svc.Name, svc.Preset)
			if err != nil {
				errs = append(errs, err)
			} else if applied {
				res.ConfigsApplied = append(res.ConfigsApplied, svc.Name)
			} else if unitChanged || containerOlderThanUnit(svc.Name) {
				// The drift check reads the materialised config files and is blind to
				// the unit itself, so a definition that moved a port, changed the image
				// or added an environment variable would sit rewritten on disk while the
				// container kept running on what it started with.
				if restarted, err := restartRunningUnit(svc.Name); err != nil {
					errs = append(errs, err)
				} else if restarted {
					res.ConfigsApplied = append(res.ConfigsApplied, svc.Name)
				}
			}
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
	// An install made from the preset's single image recorded no version; the
	// canonical one it runs is written back here, once.
	version := preset.InstalledVersion(svc)
	// ResolvePinned pins the canonical instance to its version's port, but only a
	// multi-version preset has versions to pin; a single-version preset (no
	// versions list) must go through Resolve or it never receives store refreshes.
	if svc.Name == preset.Name && len(preset.Versions) > 0 {
		fresh, err = preset.ResolvePinned(version)
	} else {
		fresh, err = preset.Resolve(version)
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
	fresh.Plugin = svc.Plugin
	fresh.PresetVersion = version
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

// restartRunningUnit restarts a service only if its container is up, so a
// rewritten unit reaches a running service without starting a stopped one.
func restartRunningUnit(name string) (bool, error) {
	if _, running := containerStartedAtFn("lerd-" + name); !running {
		return false, nil
	}
	if err := restartUnitFn("lerd-" + name); err != nil {
		return false, fmt.Errorf("restarting %s after a unit change: %w", name, err)
	}
	return true, nil
}

var ensureDefaultQuadletFn = ensureDefaultPresetQuadletDiff

// ReconcileDefaultService is ReconcileServices for one installed default
// service (mysql, redis...), which keeps no YAML of its own: re-render its unit
// from the preset and restart a running container that predates the change.
func ReconcileDefaultService(name string) (bool, error) {
	if !UnitInstalledFn("lerd-" + name) {
		return false, nil
	}
	changed, err := ensureDefaultQuadletFn(name)
	if err != nil {
		return false, fmt.Errorf("regenerating quadlet for %s: %w", name, err)
	}
	if applied, err := RestartIfConfigDrifted(name, name); err != nil || applied {
		return applied, err
	}
	if !changed && !containerOlderThanUnit(name) {
		return false, nil
	}
	return restartRunningUnit(name)
}
