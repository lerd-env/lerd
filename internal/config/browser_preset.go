package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

// PackageBrowser is what a package definition declares for browser capture:
// the DOM events its frontend library fires when something fails, under the
// preset they are offered as, which packages of one library share.
type PackageBrowser struct {
	Preset string                `yaml:"preset"`
	Label  string                `yaml:"label"`
	Events []BrowserCaptureEvent `yaml:"events"`
}

// BrowserPreset is a set of browser capture events for a frontend library,
// gathered from the store packages that declare it, with the composer and npm
// packages that show a project uses it. Applying one copies its events into
// the site's own settings.
type BrowserPreset struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Detect struct {
		Composer []string `json:"composer,omitempty"`
		NPM      []string `json:"npm,omitempty"`
	} `json:"detect"`
	Events []BrowserCaptureEvent `json:"events"`
}

// BrowserPresets returns the presets the store's packages declare for the
// project at dir: every npm package's, since there are few and a site may add
// one it does not use yet, and those of the composer packages it has installed,
// so a listing never fetches every package file. Events lerd would refuse from
// a site are dropped.
func BrowserPresets(dir string) []BrowserPreset {
	byName := map[string]*BrowserPreset{}
	var order []string
	add := func(entry StorePackageEntry, npm bool) {
		want := PackageComposer
		if npm {
			want = PackageNPM
		}
		pkg := loadStorePackage(entry.Name, pickPackageVersion(dir, entry))
		if pkg == nil || pkg.Browser == nil || !packageTypeIs(pkg, want) || !browserPresetNameRE.MatchString(pkg.Browser.Preset) {
			return
		}
		b := pkg.Browser
		p := byName[b.Preset]
		if p == nil {
			p = &BrowserPreset{Name: b.Preset, Label: b.Label, Events: []BrowserCaptureEvent{}}
			byName[b.Preset] = p
			order = append(order, b.Preset)
		}
		if npm {
			p.Detect.NPM = append(p.Detect.NPM, pkg.Package)
		} else {
			p.Detect.Composer = append(p.Detect.Composer, pkg.Package)
		}
		for _, e := range b.Events {
			if e.valid() && !slices.ContainsFunc(p.Events, func(o BrowserCaptureEvent) bool { return o.Event == e.Event }) {
				p.Events = append(p.Events, e)
			}
		}
	}
	for _, e := range cachedStoreNPMPackages() {
		add(e, true)
	}
	if dir != "" {
		for _, e := range cachedStorePackages() {
			if ComposerHasInstalled(dir, e.Name) {
				add(e, false)
			}
		}
	}
	slices.Sort(order)
	out := make([]BrowserPreset, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}

// FindBrowserPreset returns one of the presets offered for the project at dir.
func FindBrowserPreset(dir, name string) (BrowserPreset, bool) {
	for _, p := range BrowserPresets(dir) {
		if p.Name == name {
			return p, true
		}
	}
	return BrowserPreset{}, false
}

var browserPresetNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

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
