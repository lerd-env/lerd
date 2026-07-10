package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func names(proj *config.ProjectConfig) []string {
	out := []string{}
	for _, s := range proj.Services {
		out = append(out, s.Name)
	}
	return out
}

func TestEnsureRequiredServicesAddsMissingPreset(t *testing.T) {
	dir := t.TempDir()
	proj := &config.ProjectConfig{Services: []config.ProjectService{{Name: "mysql"}}}
	fw := &config.Framework{Label: "Magento", Requires: []string{"opensearch"}}

	got := ensureRequiredServices(dir, proj, fw, func(string) bool { return true })

	if len(got.Services) != 2 {
		t.Fatalf("services = %v", names(got))
	}
	added := got.Services[1]
	if added.Name != "opensearch" || added.Preset != "opensearch" {
		t.Fatalf("added = %+v, want a preset reference", added)
	}
	// Persisted, so a teammate cloning the repo gets the same service.
	reloaded, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("project config not saved: %v", err)
	}
	if len(reloaded.Services) != 2 {
		t.Fatalf("saved services = %v", names(reloaded))
	}
}

func TestEnsureRequiredServicesIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	proj := &config.ProjectConfig{Services: []config.ProjectService{
		{Name: "opensearch", Preset: "opensearch"},
	}}
	fw := &config.Framework{Requires: []string{"opensearch"}}

	got := ensureRequiredServices(dir, proj, fw, func(string) bool { return true })
	if len(got.Services) != 1 {
		t.Fatalf("duplicated the service: %v", names(got))
	}
}

// A default preset (mysql, redis) is referenced by bare name, not as a preset
// reference, matching what the init wizard writes.
func TestEnsureRequiredServicesDefaultPresetIsBareName(t *testing.T) {
	dir := t.TempDir()
	fw := &config.Framework{Requires: []string{"mysql"}}
	got := ensureRequiredServices(dir, &config.ProjectConfig{}, fw, func(string) bool { return true })
	if len(got.Services) != 1 || got.Services[0].Preset != "" {
		t.Fatalf("got %+v, want a bare name", got.Services)
	}
}

// A definition naming a service the store has never heard of must not write a
// broken entry into the project's committed config.
func TestEnsureRequiredServicesSkipsUnknownPreset(t *testing.T) {
	dir := t.TempDir()
	fw := &config.Framework{Requires: []string{"nonexistent-engine"}}
	got := ensureRequiredServices(dir, &config.ProjectConfig{}, fw, func(string) bool { return false })
	if len(got.Services) != 0 {
		t.Fatalf("wrote an unknown service: %v", names(got))
	}
}

func TestEnsureRequiredServicesNoRequiresIsNoop(t *testing.T) {
	dir := t.TempDir()
	proj := &config.ProjectConfig{}
	if got := ensureRequiredServices(dir, proj, &config.Framework{}, func(string) bool { return true }); len(got.Services) != 0 {
		t.Fatal("added services for a framework that requires none")
	}
	if got := ensureRequiredServices(dir, proj, nil, func(string) bool { return true }); got != proj {
		t.Fatal("nil framework should return the project untouched")
	}
	// Nothing to save, so no .lerd.yaml should appear on disk.
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); err == nil {
		t.Error("wrote .lerd.yaml with nothing to add")
	}
}

func TestEnsureRequiredServicesNilProject(t *testing.T) {
	dir := t.TempDir()
	fw := &config.Framework{Requires: []string{"opensearch"}}
	got := ensureRequiredServices(dir, nil, fw, func(string) bool { return true })
	if got == nil || len(got.Services) != 1 {
		t.Fatalf("got %+v", got)
	}
}
