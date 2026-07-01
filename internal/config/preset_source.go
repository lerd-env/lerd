package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// presetStaleAfter is how long a cached store preset is served before EnsurePreset
// opportunistically refreshes it from the store, mirroring the framework store's
// 24h window.
const presetStaleAfter = 24 * time.Hour

// PresetFetchFunc downloads a preset from the external service store and saves it
// into the store-cache dir. The store package registers it at startup to avoid a
// circular import (config must not import store).
type PresetFetchFunc func(name string) error

var presetFetchHook PresetFetchFunc

// RegisterPresetFetchHook sets the callback used to auto-fetch missing service
// presets from the store.
func RegisterPresetFetchHook(fn PresetFetchFunc) { presetFetchHook = fn }

// EnsurePreset returns the preset for name, fetching it from the external store
// when no local copy exists and opportunistically refreshing a cached store
// preset older than presetStaleAfter. This is the deliberate, network-touching
// entry point used by install and the service CLI. The plain LoadPreset /
// PresetExists paths stay purely local so hot loops and the many "is this name a
// preset?" checks never reach out to the network.
func EnsurePreset(name string) (*Preset, error) {
	if !validPresetName(name) {
		return nil, fmt.Errorf("invalid preset name %q", name)
	}
	switch {
	case storePresetStale(name):
		// Best-effort refresh: the cached copy still serves if the store is down.
		if presetFetchHook != nil {
			_ = presetFetchHook(name)
		}
	case !presetSourceExists(name):
		if presetFetchHook == nil {
			return nil, fmt.Errorf("unknown preset %q", name)
		}
		if err := presetFetchHook(name); err != nil {
			return nil, fmt.Errorf("unknown preset %q: not bundled and fetching it from the store failed: %w", name, err)
		}
	}
	return LoadPreset(name)
}

// storePresetStale reports whether name has a cached store preset older than the
// staleness window. A built-in served only from the embed bundle (no cache file)
// is never "stale": built-ins refresh through an explicit `lerd service update`,
// not implicitly on every install.
func storePresetStale(name string) bool {
	info, err := os.Stat(filepath.Join(StorePresetsDir(), name+".yaml"))
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > presetStaleAfter
}

// preset_source.go is the single seam through which every service preset is
// read. All callers (LoadPreset, ListPresets, PresetExists, the default-preset
// and family indexes) go through presetNames / readPresetBytes / presetSource
// Exists so later phases can layer a local store cache and a remote fetch
// underneath without touching any call site.
//
// The embedded bundle is always the last layer and is never removed: it is the
// offline fallback that guarantees services installed by an older lerd keep
// resolving by name after their preset has moved to the external store.

// extraPresetsFS is an optional preset source layered under the store cache and
// above the embedded bundle. It is nil in production; tests set it (via
// SetExtraPresetsForTest) to the add-on presets that no longer ship embedded, so
// existing tests keep resolving them without a network fetch.
var extraPresetsFS fs.FS

// SetExtraPresetsForTest installs (or clears, with nil) the test-only extra
// preset layer. Production never calls this, so the shipped binary serves only
// the embedded defaults plus whatever the store fetches into the cache.
func SetExtraPresetsForTest(fsys fs.FS) { extraPresetsFS = fsys }

// readPresetBytes returns the raw YAML for a preset name and whether it was
// found. The store-cache dir is checked first so a fetched preset can supersede
// the built-in of the same name; a cache file that fails validation is ignored
// so a bad store entry can never shadow a working built-in. Next the test-only
// extra layer, then the embedded bundle as the final, always-present fallback
// (offline + backwards compatibility).
func readPresetBytes(name string) ([]byte, bool) {
	if !validPresetName(name) {
		return nil, false
	}
	if data, err := os.ReadFile(filepath.Join(StorePresetsDir(), name+".yaml")); err == nil {
		if ValidatePresetYAML(data, name) == nil {
			return data, true
		}
	}
	if extraPresetsFS != nil {
		if data, err := fs.ReadFile(extraPresetsFS, name+".yaml"); err == nil {
			return data, true
		}
	}
	data, err := presetFS.ReadFile("presets/" + name + ".yaml")
	if err != nil {
		return nil, false
	}
	return data, true
}

// validPresetName rejects names that could escape the preset directories via
// path separators or traversal, since names index straight into file paths.
func validPresetName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/\\") && name != "." && name != ".."
}

// SaveStorePreset validates raw preset YAML fetched from the external service
// store and writes it verbatim into the store-cache dir, where the seam then
// serves it above the embedded bundle. Writing the original bytes (rather than a
// re-marshal) preserves every field, including any the running binary's Preset
// struct doesn't yet know about. The name is taken from the argument, not the
// YAML, so it always matches the on-disk filename the seam looks up.
func SaveStorePreset(name string, data []byte) error {
	if !validPresetName(name) {
		return fmt.Errorf("invalid preset name %q", name)
	}
	if err := ValidatePresetYAML(data, name); err != nil {
		return err
	}
	dir := StorePresetsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), data, 0o644); err != nil {
		return err
	}
	presetCache.Delete(name)
	return nil
}

// RemoveStorePreset deletes a preset's cached copy from the store-cache dir, if
// present, and drops it from the parse cache. It never touches the embedded
// bundle (a no-op when only the built-in exists). Called when the last service
// using a store preset is removed, so the definition reverts from "local" to
// "store" and a future install re-fetches it fresh.
func RemoveStorePreset(name string) error {
	if !validPresetName(name) {
		return nil
	}
	err := os.Remove(filepath.Join(StorePresetsDir(), name+".yaml"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	presetCache.Delete(name)
	return nil
}

// presetSourceExists reports whether any layer can serve a preset by this name.
func presetSourceExists(name string) bool {
	_, ok := readPresetBytes(name)
	return ok
}

// presetNames returns the sorted, de-duplicated set of every preset name the
// source can serve across all layers. The union means a store-only preset is
// treated as a first-class preset by the default-preset and family indexes.
func presetNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(names []string) {
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	add(embeddedPresetNames())
	add(storePresetNames())
	add(extraPresetNames())
	sort.Strings(out)
	return out
}

// extraPresetNames lists names in the test-only extra preset layer (nil in prod).
func extraPresetNames() []string {
	if extraPresetsFS == nil {
		return nil
	}
	entries, err := fs.ReadDir(extraPresetsFS, ".")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return out
}

// storePresetNames lists preset names present in the store-cache dir. Only
// syntactically valid names are returned; content validation happens lazily in
// readPresetBytes so an enumerated name that later fails to parse simply falls
// back to the embedded bundle (or is dropped if there is no built-in).
func storePresetNames() []string {
	entries, err := os.ReadDir(StorePresetsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		if validPresetName(name) {
			out = append(out, name)
		}
	}
	return out
}

// embeddedPresetNames lists the preset names shipped in the binary.
func embeddedPresetNames() []string {
	entries, err := fs.ReadDir(presetFS, "presets")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return out
}
