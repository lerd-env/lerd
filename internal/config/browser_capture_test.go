package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBrowserCaptureResolve_Defaults(t *testing.T) {
	var b *BrowserCapture
	got := b.Resolve()
	want := BrowserCaptureSettings{Enabled: true, Console: []string{"error"}, Network: []string{}, Navigation: true, Events: []BrowserCaptureEvent{}, Presets: []string{}, Route: DefaultBrowserCaptureRoute}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestBrowserCaptureRoute_OwnRouteAndUnsafeOnesRefused(t *testing.T) {
	if got := (&BrowserCapture{Route: "/__dev/js-errors"}).Resolve().Route; got != "/__dev/js-errors" {
		t.Fatalf("Route = %q", got)
	}
	for _, bad := range []string{"/", "_lerd", "/a/../b", "/a b", "/a;deny all", "/a/"} {
		if err := (BrowserCaptureSettings{Route: bad}).Validate(); err == nil {
			t.Errorf("route %q accepted", bad)
		}
		if got := (&BrowserCapture{Route: bad}).Resolve().Route; got != DefaultBrowserCaptureRoute {
			t.Errorf("route %q resolved to %q, want the default", bad, got)
		}
	}
}

func TestBrowserCaptureResolve_ExplicitEmptyListsAndUnknownValues(t *testing.T) {
	off := false
	got := (&BrowserCapture{Enabled: &off, Console: []string{}, Network: []string{"5xx", "3xx", "5xx"}}).Resolve()
	want := BrowserCaptureSettings{Enabled: false, Console: []string{}, Network: []string{"5xx"}, Navigation: true, Events: []BrowserCaptureEvent{}, Presets: []string{}, Route: DefaultBrowserCaptureRoute}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestBrowserCaptureSettingsValidate_NamesUnknownValue(t *testing.T) {
	err := BrowserCaptureSettings{Console: []string{"info"}}.Validate()
	if err == nil || !strings.Contains(err.Error(), `"info"`) {
		t.Fatalf("Validate() = %v, want an error naming \"info\"", err)
	}
}

// Without a .lerd.yaml the settings land in the registry and no file is created.
func TestSaveBrowserCapture_RegistryWhenNoProjectFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := t.TempDir()
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatal(err)
	}
	s, _ := FindSite("shop")
	want := BrowserCaptureSettings{Enabled: true, Console: []string{"error", "warn"}, Network: []string{"4xx"}, Navigation: false, Events: []BrowserCaptureEvent{{Event: "inertia:invalid", Label: "Inertia", Message: "detail.response.status"}}, Presets: []string{"inertia"}, Route: "/__dev/capture"}
	if err := SaveBrowserCapture(*s, want); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); !os.IsNotExist(err) {
		t.Fatalf(".lerd.yaml was created (stat err %v)", err)
	}
	invalidateSitesCache()
	s, _ = FindSite("shop")
	if got := BrowserCaptureFor(*s); !reflect.DeepEqual(got, want) {
		t.Fatalf("BrowserCaptureFor = %+v, want %+v", got, want)
	}
}

// With a .lerd.yaml the settings are written there and it wins over the registry.
func TestSaveBrowserCapture_ProjectFileWhenPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("php_version: \"8.4\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	off := false
	site := Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir, BrowserCapture: &BrowserCapture{Enabled: &off}}
	want := BrowserCaptureSettings{Enabled: true, Console: []string{}, Network: []string{"5xx"}, Navigation: true, Events: []BrowserCaptureEvent{}, Presets: []string{}, Route: DefaultBrowserCaptureRoute}
	if err := SaveBrowserCapture(site, want); err != nil {
		t.Fatal(err)
	}
	if got := BrowserCaptureFor(site); !reflect.DeepEqual(got, want) {
		t.Fatalf("BrowserCaptureFor = %+v, want %+v", got, want)
	}
	proj, _ := LoadProjectConfig(dir)
	if proj.PHPVersion != "8.4" {
		t.Fatalf("php_version lost on save: %q", proj.PHPVersion)
	}
}

func TestBrowserCaptureEvents_OnlyNamesAndPaths(t *testing.T) {
	good := BrowserCaptureEvent{Event: "htmx:responseError", Message: "detail.xhr.status"}
	if err := (BrowserCaptureSettings{Events: []BrowserCaptureEvent{good}}).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []BrowserCaptureEvent{
		{Event: "x</script><script>alert(1)"},
		{Event: "ok", Message: "detail.a()"},
		{Event: "ok", Message: "detail[0]"},
		{Event: ""},
	} {
		if err := (BrowserCaptureSettings{Events: []BrowserCaptureEvent{bad}}).Validate(); err == nil {
			t.Errorf("event %+v accepted", bad)
		}
		if got := (&BrowserCapture{Events: []BrowserCaptureEvent{bad, good}}).Resolve().Events; len(got) != 1 || got[0] != good {
			t.Errorf("event %+v resolved to %+v", bad, got)
		}
	}
}
