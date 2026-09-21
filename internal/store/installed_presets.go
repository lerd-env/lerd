package store

import "github.com/geodro/lerd/internal/config"

// ensurePresetFn is the refresh of one preset, a seam so the sweep can be
// tested without a store. EnsurePreset fetches a preset the cache does not have
// and re-reads one older than the staleness window, leaving a fresh copy and an
// embedded default alone.
var ensurePresetFn = func(name string) error {
	_, err := config.EnsurePreset(name)
	return err
}

// RefreshInstalledPresets re-reads the store definition of every installed
// service that came from one. A preset is otherwise fetched the once, when the
// service is installed, so a definition the store has changed since, a dashboard
// it has gained or the copy rendered for a newer schema, never reaches a host
// already running the service. Best effort: this refreshes something that
// already works, so an unreachable store simply changes nothing. Returns how
// many definitions it refreshed.
//
// `lerd install` and `lerd update` do the same walk loudly and unconditionally
// (cli.refreshStorePresets); this is the quiet one between those runs, which is
// where a host that only ever swaps binaries would otherwise never look.
func RefreshInstalledPresets() int {
	svcs, err := config.ListCustomServices()
	if err != nil {
		return 0
	}
	seen := map[string]bool{}
	var refreshed int
	for _, svc := range svcs {
		if svc == nil || svc.Preset == "" || seen[svc.Preset] {
			continue
		}
		seen[svc.Preset] = true
		if ensurePresetFn(svc.Preset) == nil {
			refreshed++
		}
	}
	return refreshed
}
