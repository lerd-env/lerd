package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// seedBrowserPackages publishes the store index and the package files that
// declare browser events: Inertia through its composer adapter and one of its
// npm clients, htmx through npm alone, and an entry of a type lerd does not know.
func seedBrowserPackages(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	prev := packageFetchHook
	packageFetchHook = nil
	t.Cleanup(func() { packageFetchHook = prev })
	if err := os.MkdirAll(filepath.Dir(StoreIndexFile()), 0755); err != nil {
		t.Fatal(err)
	}
	index := `{"frameworks":[],"packages":[{"name":"inertiajs/inertia-laravel"}],"npm_packages":[{"name":"@inertiajs/vue3"},{"name":"htmx.org"},{"name":"left-pad"}]}`
	if err := os.WriteFile(StoreIndexFile(), []byte(index), 0644); err != nil {
		t.Fatal(err)
	}
	inertia := func(pkg, typ string) *FrameworkPackage {
		return &FrameworkPackage{Package: pkg, Type: typ, Browser: &PackageBrowser{Preset: "inertia", Label: "Inertia.js", Events: []BrowserCaptureEvent{{Event: "inertia:invalid", Message: "detail.response.status"}, {Event: "bad event()"}}}}
	}
	for _, p := range []*FrameworkPackage{
		inertia("inertiajs/inertia-laravel", ""),
		inertia("@inertiajs/vue3", PackageNPM),
		{Package: "htmx.org", Type: PackageNPM, Browser: &PackageBrowser{Preset: "htmx", Label: "htmx", Events: []BrowserCaptureEvent{{Event: "htmx:responseError"}}}},
		{Package: "left-pad", Type: "pip", Browser: &PackageBrowser{Preset: "pad", Label: "pad", Events: []BrowserCaptureEvent{{Event: "pad:error"}}}},
	} {
		if err := SaveStorePackage(p); err != nil {
			t.Fatal(err)
		}
	}
}

func project(t *testing.T, composer, npm string) string {
	t.Helper()
	dir := t.TempDir()
	if composer != "" {
		os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composer), 0644) //nolint:errcheck
	}
	if npm != "" {
		os.WriteFile(filepath.Join(dir, "package.json"), []byte(npm), 0644) //nolint:errcheck
	}
	return dir
}

// Packages that share a preset are offered as one, detected by any of them,
// and an event lerd would refuse from a site is dropped from it.
func TestBrowserPresets_GroupThePackagesThatShareOne(t *testing.T) {
	seedBrowserPackages(t)
	dir := project(t, `{"require":{"inertiajs/inertia-laravel":"^2.0"}}`, "")
	got := BrowserPresets(dir)
	if len(got) != 2 || got[0].Name != "htmx" || got[1].Name != "inertia" {
		t.Fatalf("BrowserPresets = %+v", got)
	}
	in := got[1]
	if in.Label != "Inertia.js" || len(in.Events) != 1 || len(in.Detect.Composer) != 1 || len(in.Detect.NPM) != 1 {
		t.Errorf("inertia = %+v", in)
	}
}

// An npm package's preset is always offered; a composer package's only where
// the project has it, so listing presets never fetches every package file.
func TestBrowserPresets_ComposerOnlyWhereInstalled(t *testing.T) {
	seedBrowserPackages(t)
	got := BrowserPresets(t.TempDir())
	if len(got) != 2 {
		t.Fatalf("BrowserPresets = %+v", got)
	}
	for _, p := range got {
		if p.Name == "inertia" && len(p.Detect.Composer) != 0 {
			t.Errorf("composer package offered without being installed: %+v", p.Detect)
		}
	}
}

func TestBrowserPreset_DetectedFromComposerOrNPM(t *testing.T) {
	seedBrowserPackages(t)
	both := project(t, `{"require":{"inertiajs/inertia-laravel":"^2.0"}}`, `{"devDependencies":{"htmx.org":"^2"}}`)
	for _, p := range BrowserPresets(both) {
		if !p.Detected(both) {
			t.Errorf("%s not detected", p.Name)
		}
		if p.Detected(t.TempDir()) {
			t.Errorf("%s detected in an empty project", p.Name)
		}
	}
	vue := project(t, "", `{"dependencies":{"@inertiajs/vue3":"^2"}}`)
	if p, ok := FindBrowserPreset(vue, "inertia"); !ok || !p.Detected(vue) {
		t.Errorf("inertia not detected through its npm client: %+v", p)
	}
}

// A package whose file declares a type lerd does not know is never read as one.
func TestBrowserPresets_SkipAPackageOfUnknownType(t *testing.T) {
	seedBrowserPackages(t)
	if _, ok := FindBrowserPreset("", "pad"); ok {
		t.Error("a preset from a package of unknown type was offered")
	}
}

func TestPackageSlug_NamesComposerAndNPMFilesApart(t *testing.T) {
	for name, want := range map[string]string{
		"laravel/horizon":  "laravel-horizon",
		"@inertiajs/vue3":  "npm-inertiajs-vue3",
		"vite":             "npm-vite",
		"htmx.org":         "npm-htmx.org",
		"../etc":           "",
		"@x/../y":          "",
		"Laravel/Horizon":  "",
		"@inertiajs/vue3/": "",
	} {
		if got := PackageSlug(name); got != want {
			t.Errorf("PackageSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

// A detected preset is on until the site switches it off; any other preset
// is off until the site switches it on.
func TestBrowserPreset_ActiveIsDetectedUnlessOverridden(t *testing.T) {
	p := BrowserPreset{Name: "inertia"}
	for _, tc := range []struct {
		name     string
		presets  map[string]bool
		detected bool
		want     bool
	}{
		{"detected, no override", nil, true, true},
		{"not detected, no override", nil, false, false},
		{"detected, switched off", map[string]bool{"inertia": false}, true, false},
		{"not detected, switched on", map[string]bool{"inertia": true}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Active(BrowserCaptureSettings{Presets: tc.presets}, tc.detected); got != tc.want {
				t.Fatalf("Active = %v, want %v", got, tc.want)
			}
		})
	}
}

// The script reports the site's own events plus every active preset's, once
// each, with the site's own wording kept when both name the same event.
func TestPageEvents_SiteEventsThenActivePresets(t *testing.T) {
	inertia := BrowserPreset{Name: "inertia", Events: []BrowserCaptureEvent{{Event: "inertia:invalid", Label: "store"}, {Event: "inertia:exception"}}}
	turbo := BrowserPreset{Name: "turbo", Events: []BrowserCaptureEvent{{Event: "turbo:frame-missing"}}}
	s := BrowserCaptureSettings{Events: []BrowserCaptureEvent{{Event: "inertia:invalid", Label: "mine"}}}
	got := PageEvents(s, []BrowserPreset{inertia, turbo}, func(p BrowserPreset) bool { return p.Name == "inertia" })
	want := []BrowserCaptureEvent{{Event: "inertia:invalid", Label: "mine"}, {Event: "inertia:exception"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PageEvents = %+v, want %+v", got, want)
	}
}

// A preset without events still lists an empty array, so the dashboard reads
// every preset the same way.
func TestBrowserPresets_EmptyEventsListNotNull(t *testing.T) {
	seedBrowserPackages(t)
	if err := SaveStorePackage(&FrameworkPackage{Package: "htmx.org", Type: PackageNPM, Browser: &PackageBrowser{Preset: "htmx", Label: "htmx"}}); err != nil {
		t.Fatal(err)
	}
	p, ok := FindBrowserPreset("", "htmx")
	if !ok || p.Events == nil {
		t.Fatalf("htmx = %+v", p)
	}
}
