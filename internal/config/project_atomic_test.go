package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// SaveProjectConfig must emit two-space indentation so .lerd.yaml matches the
// hand-authored store YAML and stops looking self-inconsistent.
func TestSaveProjectConfig_TwoSpaceIndent(t *testing.T) {
	dir := t.TempDir()
	cfg := &ProjectConfig{
		Domains:  []string{"acme"},
		Services: []ProjectService{{Name: "mysql"}},
	}
	if err := SaveProjectConfig(dir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "    - acme") {
		t.Errorf("output uses four-space indent, want two-space:\n%s", data)
	}
	if !strings.Contains(string(data), "\n  - acme") {
		t.Errorf("domains list not indented two spaces:\n%s", data)
	}
}

// Workers and services persist in sorted order so a config change produces a
// minimal git diff instead of a reshuffled block.
func TestSaveProjectConfig_SortsWorkersAndServices(t *testing.T) {
	dir := t.TempDir()
	cfg := &ProjectConfig{
		Services:      []ProjectService{{Name: "redis"}, {Name: "mysql"}, {Name: "mailpit"}},
		Workers:       []string{"vite", "horizon", "schedule"},
		ReloadWorkers: []string{"vite", "horizon"},
	}
	if err := SaveProjectConfig(dir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	wantSvc := []string{"mailpit", "mysql", "redis"}
	for i, w := range wantSvc {
		if got.Services[i].Name != w {
			t.Errorf("services not sorted: got %v", serviceNames(got.Services))
			break
		}
	}
	wantWrk := []string{"horizon", "schedule", "vite"}
	for i, w := range wantWrk {
		if got.Workers[i] != w {
			t.Errorf("workers not sorted: got %v", got.Workers)
			break
		}
	}
	if got.ReloadWorkers[0] != "horizon" || got.ReloadWorkers[1] != "vite" {
		t.Errorf("reload_workers not sorted: got %v", got.ReloadWorkers)
	}
}

// A crash, a restart mid-write, or two concurrent writers must never leave a
// half-written .lerd.yaml or a stray temp file behind: the atomic write means a
// reader always sees a complete, parseable file.
func TestSaveProjectConfig_AtomicNoTempLeftAndAlwaysValid(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cfg := &ProjectConfig{
				Domains: []string{"acme"},
				Workers: []string{"horizon", "schedule", "vite"},
			}
			_ = SaveProjectConfig(dir, cfg)
			// A reader racing the writers must never see a corrupt file.
			if _, err := LoadProjectConfig(dir); err != nil {
				t.Errorf("read during concurrent writes saw corrupt file: %v", err)
			}
		}(i)
	}
	wg.Wait()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if _, err := LoadProjectConfig(dir); err != nil {
		t.Fatalf("final file is not valid: %v", err)
	}
}

// AddProjectWorker is the only additive path into the workers list, so it must
// refuse a whitespace-bearing name that would otherwise land as a single
// mangled element like "horizon - schedule - vite - stripe".
func TestAddProjectWorker_RejectsWhitespaceName(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProjectConfig(dir, &ProjectConfig{Workers: []string{"horizon"}}); err != nil {
		t.Fatal(err)
	}
	if err := AddProjectWorker(dir, "horizon - schedule - vite - stripe"); err == nil {
		t.Error("expected an error for a whitespace-bearing worker name")
	}
	got, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range got.Workers {
		if strings.ContainsAny(w, " \t\n") {
			t.Errorf("mangled worker name persisted: %q", w)
		}
	}
	if len(got.Workers) != 1 {
		t.Errorf("workers list changed: %v", got.Workers)
	}
}

func serviceNames(s []ProjectService) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = v.Name
	}
	return out
}
