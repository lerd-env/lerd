package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestStarterSupports_followsTheFrameworkRange(t *testing.T) {
	s := Starter{Framework: FrameworkPackageScope{Name: "laravel", Min: "11", Max: "12"}}
	for v, want := range map[string]bool{"10": false, "11": true, "12": true, "13": false, "": false} {
		if got := s.Supports(v); got != want {
			t.Errorf("Supports(%q) = %v, want %v", v, got, want)
		}
	}
	open := Starter{Framework: FrameworkPackageScope{Name: "laravel"}}
	if !open.Supports("6") {
		t.Error("a starter with no range should support every major")
	}
}

func TestStarterValidate(t *testing.T) {
	ok := Starter{Starter: "filament", Label: "Filament", Framework: FrameworkPackageScope{Name: "laravel"}, Require: []string{"filament/filament", "livewire/flux:^2.0"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid starter refused: %v", err)
	}
	for name, s := range map[string]Starter{
		"no name":        {Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}},
		"bad name":       {Starter: "../x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}},
		"no label":       {Starter: "x", Framework: FrameworkPackageScope{Name: "laravel"}},
		"no framework":   {Starter: "x", Label: "x"},
		"nothing to do":  {Starter: "x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}},
		"flag":           {Starter: "x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}, Require: []string{"--dev"}},
		"spaced":         {Starter: "x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}, Require: []string{"filament/filament ^4.0"}},
		"not a package":  {Starter: "x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}, Require: []string{"filament"}},
		"empty version":  {Starter: "x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}, Require: []string{"filament/filament:"}},
		"create not cmd": {Starter: "x", Label: "x", Framework: FrameworkPackageScope{Name: "laravel"}, Create: "  "},
	} {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: expected Validate to refuse %+v", name, s)
		}
	}
}

// A skeleton created with --no-install has no lock, and resolving one at create
// time would pin it to the scaffold's PHP, so the require only edits the manifest.
// A skeleton that installed its vendor has a lock the require has to keep current.
func TestStarterRequireArgs_updatesOnlyWhenTheSkeletonIsInstalled(t *testing.T) {
	s := Starter{Require: []string{"filament/filament"}}
	dir := t.TempDir()

	got := s.RequireArgs(dir)
	if !slices.Contains(got, "--no-update") || got[0] != "require" || got[len(got)-1] != "filament/filament" {
		t.Errorf("RequireArgs without a lock = %v", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "composer.lock"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := s.RequireArgs(dir); slices.Contains(got, "--no-update") {
		t.Errorf("RequireArgs with a lock = %v, should update it", got)
	}
}

func TestStoreStarters_readsTheFrameworksStartersFromTheCachedIndex(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeCachedIndex(t, `{"frameworks":[],"starters":[
		{"name":"filament","label":"Filament","framework":{"name":"laravel","min":"11"}},
		{"name":"webapp","label":"Web app","framework":{"name":"symfony"}},
		{"name":"react","label":"React","framework":{"name":"laravel","min":"13"}}
	]}`)

	got := StoreStarters("laravel")
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "filament,react" {
		t.Errorf("StoreStarters(laravel) = %v", names)
	}
	if !got[0].Supports("11") || got[1].Supports("12") {
		t.Error("index entries should carry the framework range")
	}
}

func writeCachedIndex(t *testing.T, body string) {
	t.Helper()
	path := StoreIndexFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
