package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugbar_ResolveFillsDefaultsAndDropsUnknown(t *testing.T) {
	got := Debugbar{Style: "compact", Edge: "sideways", Theme: "dark"}.Resolve()
	want := Debugbar{Style: "compact", Edge: "bottom", Corner: "bottom-right", Theme: "dark"}
	if got != want {
		t.Fatalf("Resolve = %+v, want %+v", got, want)
	}
}

func TestDebugbar_ValidateNamesUnknownValue(t *testing.T) {
	if err := (Debugbar{Style: "dock", Corner: "top-left"}).Validate(); err != nil {
		t.Fatal(err)
	}
	err := Debugbar{Corner: "middle"}.Validate()
	if err == nil || !strings.Contains(err.Error(), `"middle"`) {
		t.Fatalf("Validate = %v, want an error naming middle", err)
	}
}

// Without a .lerd.yaml the setting lands in the registry and no file is made.
func TestSaveDebugbar_RegistryWhenNoProjectFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := t.TempDir()
	if err := AddSite(Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir}); err != nil {
		t.Fatal(err)
	}
	s, _ := FindSite("shop")
	if err := SaveDebugbar(*s, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); !os.IsNotExist(err) {
		t.Fatalf(".lerd.yaml was created (stat err %v)", err)
	}
	invalidateSitesCache()
	s, _ = FindSite("shop")
	if !DebugbarFor(*s) {
		t.Fatal("DebugbarFor = false after saving true")
	}
}

// With a .lerd.yaml the setting is written there and wins over the registry.
func TestSaveDebugbar_ProjectFileWins(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("php_version: \"8.4\"\ndevtools:\n  exclude_commands: [inspire]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	site := Site{Name: "shop", Path: dir, Debugbar: true}
	if err := SaveDebugbar(site, false); err != nil {
		t.Fatal(err)
	}
	if DebugbarFor(site) {
		t.Fatal("DebugbarFor = true, want the .lerd.yaml's false")
	}
	proj, _ := LoadProjectConfig(dir)
	if proj.PHPVersion != "8.4" || len(proj.Devtools.ExcludeCommands) != 1 {
		t.Fatalf("other settings lost on save: %+v", proj)
	}
}
