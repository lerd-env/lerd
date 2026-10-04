package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"
)

// BrowserPreset is a store-published set of browser capture events for a
// frontend library, with the composer and npm packages that show a project
// uses it. Applying one copies its events into the site's own settings.
type BrowserPreset struct {
	Name   string `yaml:"name" json:"name"`
	Label  string `yaml:"label" json:"label"`
	Detect struct {
		Composer []string `yaml:"composer,omitempty" json:"composer,omitempty"`
		NPM      []string `yaml:"npm,omitempty" json:"npm,omitempty"`
	} `yaml:"detect" json:"detect"`
	Events []BrowserCaptureEvent `yaml:"events" json:"events"`
}

// StoreBrowserPresetEntry is one preset the store index lists.
type StoreBrowserPresetEntry struct {
	Name string `json:"name"`
}

// BrowserPresetFetchFunc downloads a preset from the store and caches it.
type BrowserPresetFetchFunc func(name string) (*BrowserPreset, error)

var browserPresetFetchHook BrowserPresetFetchFunc

// RegisterBrowserPresetFetchHook sets the callback used to fetch presets.
func RegisterBrowserPresetFetchHook(fn BrowserPresetFetchFunc) {
	browserPresetFetchHook = fn
}

var browserPresetNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// StoreBrowserPresetsDir holds the cached presets, a sibling of the packages.
func StoreBrowserPresetsDir() string {
	return filepath.Join(DataDir(), "browser")
}

// StoreBrowserPresetFile is the cache path for one preset, or "" for a name
// that is not a plain slug.
func StoreBrowserPresetFile(name string) string {
	if !browserPresetNameRE.MatchString(name) {
		return ""
	}
	return filepath.Join(StoreBrowserPresetsDir(), name+".yaml")
}

// SaveStoreBrowserPreset caches a fetched preset.
func SaveStoreBrowserPreset(p *BrowserPreset) error {
	path := StoreBrowserPresetFile(p.Name)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(StoreBrowserPresetsDir(), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return publishStoreFile(path, data, 0644)
}

// LoadBrowserPreset returns a preset from the cache, fetching it when missing
// or a day old. Events lerd would refuse from a site are dropped from it too.
func LoadBrowserPreset(name string) *BrowserPreset {
	path := StoreBrowserPresetFile(name)
	if path == "" {
		return nil
	}
	p := readBrowserPreset(path)
	if browserPresetFetchHook != nil && (p == nil || olderThan(path, storeRefreshWindow)) {
		if fetched, err := browserPresetFetchHook(name); err == nil && fetched != nil {
			p = fetched
		}
	}
	if p == nil || p.Name != name {
		return nil
	}
	p.Events = slices.DeleteFunc(append([]BrowserCaptureEvent{}, p.Events...), func(e BrowserCaptureEvent) bool { return !e.valid() })
	return p
}

func readBrowserPreset(path string) *BrowserPreset {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var p BrowserPreset
	if yaml.Unmarshal(data, &p) != nil || p.Name == "" {
		return nil
	}
	return &p
}

// BrowserPresets returns every preset the cached store index lists.
func BrowserPresets() []BrowserPreset {
	idx := loadCachedStoreIndex()
	if idx == nil {
		return nil
	}
	var out []BrowserPreset
	for _, e := range idx.BrowserPresets {
		if p := LoadBrowserPreset(e.Name); p != nil {
			out = append(out, *p)
		}
	}
	return out
}

// Contents lists the events the preset adds, for a one-line summary.
func (p BrowserPreset) Contents() []string {
	var out []string
	for _, e := range p.Events {
		out = append(out, e.Event)
	}
	return out
}

// Detected reports whether the project at dir requires one of the preset's
// composer or npm packages.
func (p BrowserPreset) Detected(dir string) bool {
	for _, pkg := range p.Detect.Composer {
		if ComposerHasInstalled(dir, pkg) {
			return true
		}
	}
	deps := npmDependencies(dir)
	for _, pkg := range p.Detect.NPM {
		if deps[pkg] {
			return true
		}
	}
	return false
}

// Applied reports whether the preset was added to the settings.
func (p BrowserPreset) Applied(s BrowserCaptureSettings) bool {
	return slices.Contains(s.Presets, p.Name)
}

// WithBrowserPreset returns the settings with the preset's events added, or
// removed when add is false. Adding leaves an event the site already lists as
// the site has it. Removing keeps an event another added preset in all also
// declares.
func WithBrowserPreset(s BrowserCaptureSettings, p BrowserPreset, add bool, all []BrowserPreset) BrowserCaptureSettings {
	out := s
	out.Events = slices.Clone(s.Events)
	out.Presets = slices.DeleteFunc(slices.Clone(s.Presets), func(n string) bool { return n == p.Name })
	has := func(list []BrowserCaptureEvent, name string) bool {
		return slices.ContainsFunc(list, func(e BrowserCaptureEvent) bool { return e.Event == name })
	}
	if add {
		out.Presets = append(out.Presets, p.Name)
		for _, e := range p.Events {
			if !has(out.Events, e.Event) {
				out.Events = append(out.Events, e)
			}
		}
		return out
	}
	var kept []BrowserCaptureEvent
	for _, o := range all {
		if o.Name != p.Name && slices.Contains(out.Presets, o.Name) {
			kept = append(kept, o.Events...)
		}
	}
	out.Events = slices.DeleteFunc(out.Events, func(e BrowserCaptureEvent) bool { return has(p.Events, e.Event) && !has(kept, e.Event) })
	return out
}

// npmDependencies returns the packages package.json depends on, dev ones too.
func npmDependencies(dir string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies    map[string]any `json:"dependencies"`
		DevDependencies map[string]any `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	out := map[string]bool{}
	for name := range pkg.Dependencies {
		out[name] = true
	}
	for name := range pkg.DevDependencies {
		out[name] = true
	}
	return out
}
