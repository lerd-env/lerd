package config

import (
	"os"
	"path/filepath"
	"testing"
)

func seedBrowserPresets(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	prev := browserPresetFetchHook
	browserPresetFetchHook = nil
	t.Cleanup(func() { browserPresetFetchHook = prev })
	if err := os.MkdirAll(filepath.Dir(StoreIndexFile()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StoreIndexFile(), []byte(`{"frameworks":[],"browser_presets":[{"name":"inertia"},{"name":"htmx"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*BrowserPreset{
		{Name: "inertia", Label: "Inertia", Events: []BrowserCaptureEvent{{Event: "inertia:invalid", Message: "detail.response.status"}, {Event: "bad event()"}}},
		{Name: "htmx", Label: "htmx", Events: []BrowserCaptureEvent{{Event: "htmx:responseError"}}},
	} {
		if p.Name == "inertia" {
			p.Detect.Composer = []string{"inertiajs/inertia-laravel"}
		} else {
			p.Detect.NPM = []string{"htmx.org"}
		}
		if err := SaveStoreBrowserPreset(p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBrowserPresets_ListedFromTheIndexWithUnsafeEventsDropped(t *testing.T) {
	seedBrowserPresets(t)
	got := BrowserPresets()
	if len(got) != 2 || got[0].Name != "inertia" || len(got[0].Events) != 1 {
		t.Fatalf("BrowserPresets = %+v", got)
	}
}

func TestBrowserPreset_DetectedFromComposerOrNPM(t *testing.T) {
	seedBrowserPresets(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"inertiajs/inertia-laravel":"^2.0"}}`), 0644) //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"devDependencies":{"htmx.org":"^2"}}`), 0644)             //nolint:errcheck
	for _, p := range BrowserPresets() {
		if !p.Detected(dir) {
			t.Errorf("%s not detected", p.Name)
		}
		if p.Detected(t.TempDir()) {
			t.Errorf("%s detected in an empty project", p.Name)
		}
	}
}

func TestWithBrowserPreset_AddsOnceAndRemovesOnlyItsEvents(t *testing.T) {
	p := BrowserPreset{Name: "inertia", Events: []BrowserCaptureEvent{{Event: "inertia:invalid", Label: "store"}}}
	own := BrowserCaptureEvent{Event: "my:error"}
	s := BrowserCaptureSettings{Events: []BrowserCaptureEvent{own, {Event: "inertia:invalid", Label: "mine"}}}

	added := WithBrowserPreset(s, p, true, nil)
	if len(added.Events) != 2 || added.Events[1].Label != "mine" || !p.Applied(added) || p.Applied(s) {
		t.Fatalf("add = %+v", added.Events)
	}
	removed := WithBrowserPreset(added, p, false, nil)
	if len(removed.Events) != 1 || removed.Events[0] != own || p.Applied(removed) {
		t.Fatalf("remove = %+v", removed.Events)
	}
}

func TestWithBrowserPreset_RemovingKeepsAnEventAnotherAddedPresetDeclares(t *testing.T) {
	shared := BrowserCaptureEvent{Event: "app:error"}
	a := BrowserPreset{Name: "a", Events: []BrowserCaptureEvent{shared, {Event: "a:only"}}}
	b := BrowserPreset{Name: "b", Events: []BrowserCaptureEvent{shared}}
	s := WithBrowserPreset(WithBrowserPreset(BrowserCaptureSettings{}, a, true, nil), b, true, nil)
	s = WithBrowserPreset(s, a, false, []BrowserPreset{a, b})
	if len(s.Events) != 1 || s.Events[0] != shared || a.Applied(s) || !b.Applied(s) {
		t.Fatalf("after removing a: %+v", s)
	}
}

// A preset without events still lists an empty array, so the dashboard reads
// every preset the same way.
func TestLoadBrowserPreset_EmptyListsNotNull(t *testing.T) {
	seedBrowserPresets(t)
	if err := SaveStoreBrowserPreset(&BrowserPreset{Name: "empty", Label: "Empty"}); err != nil {
		t.Fatal(err)
	}
	p := LoadBrowserPreset("empty")
	if p == nil || p.Events == nil {
		t.Fatalf("LoadBrowserPreset = %+v", p)
	}
}
