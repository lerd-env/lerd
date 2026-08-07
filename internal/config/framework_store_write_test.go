package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// storeFrameworkWithWorkers builds a definition big enough that a truncated
// write lands mid-file rather than never starting.
func storeFrameworkWithWorkers(label string, workers int) *Framework {
	fw := &Framework{
		Name:      "acme",
		Version:   "1",
		Label:     label,
		PublicDir: "public",
		Workers:   map[string]FrameworkWorker{},
	}
	for i := 0; i < workers; i++ {
		name := "worker" + strings.Repeat("x", i%40) + string(rune('a'+i%26))
		fw.Workers[name] = FrameworkWorker{
			Label:   strings.Repeat("padding ", 32),
			Command: "php artisan " + strings.Repeat("y", 200),
			Restart: "always",
		}
	}
	return fw
}

// A definition cut above the workers key still parses, so a reader of a
// half-written file gets a valid looking framework with nothing to run rather
// than an error and the fallback that would follow it.
func TestSaveStoreFramework_readerNeverSeesAPartialDefinition(t *testing.T) {
	store := storeSandbox(t)
	path := filepath.Join(store, "acme@1.yaml")

	if err := SaveStoreFramework(storeFrameworkWithWorkers("Seed", 60)); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			if err := SaveStoreFramework(storeFrameworkWithWorkers(strings.Repeat("L", i+1), 60)); err != nil {
				t.Errorf("save: %v", err)
				break
			}
		}
		close(stop)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var fw Framework
			if err := yaml.Unmarshal(data, &fw); err != nil {
				t.Errorf("reader saw an unparseable definition: %v", err)
				return
			}
			if len(fw.Workers) != 60 {
				t.Errorf("reader saw %d workers, want 60: the definition was published half written", len(fw.Workers))
				return
			}
		}
	}()

	wg.Wait()
}

// Concurrent writers must publish one writer's definition whole, never a blend
// of two. The store has several: the fetch hook fires from the watcher, the
// dashboard poll and the vhost renderer, and the CLI reaches it from link and
// framework add.
func TestSaveStoreFramework_concurrentWritersNeverPublishAMerge(t *testing.T) {
	store := storeSandbox(t)
	path := filepath.Join(store, "acme@1.yaml")

	labels := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot"}
	var wg sync.WaitGroup
	for _, label := range labels {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				if err := SaveStoreFramework(storeFrameworkWithWorkers(label, 60)); err != nil {
					t.Errorf("save %s: %v", label, err)
					return
				}
			}
		}(label)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fw Framework
	if err := yaml.Unmarshal(data, &fw); err != nil {
		t.Fatalf("published definition does not parse: %v", err)
	}
	found := false
	for _, label := range labels {
		if fw.Label == label {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("published label %q belongs to no single writer", fw.Label)
	}
	if len(fw.Workers) != 60 {
		t.Errorf("published definition has %d workers, want 60", len(fw.Workers))
	}
}

// The temp a write goes through must not survive it, or the store dir fills
// with debris that the framework listing then has to ignore.
func TestSaveStoreFramework_leavesNoTempFileBehind(t *testing.T) {
	store := storeSandbox(t)

	for i := 0; i < 3; i++ {
		if err := SaveStoreFramework(storeFrameworkWithWorkers("Acme", 10)); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "acme@1.yaml" {
			t.Errorf("unexpected leftover in the store dir: %s", e.Name())
		}
	}
}

// The unversioned filename is what older installs read, so it has to keep
// working alongside the versioned one.
func TestSaveStoreFramework_writesTheUnversionedNameWhenVersionIsEmpty(t *testing.T) {
	store := storeSandbox(t)

	fw := storeFrameworkWithWorkers("Acme", 4)
	fw.Version = ""
	if err := SaveStoreFramework(fw); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(store, "acme.yaml")); err != nil {
		t.Errorf("unversioned definition not written: %v", err)
	}
}
