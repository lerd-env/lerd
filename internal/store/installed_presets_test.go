package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func writeInstalledService(t *testing.T, name, preset string) {
	t.Helper()
	dir := config.CustomServicesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "name: " + name + "\nimage: example/" + name + ":1\n"
	if preset != "" {
		body += "preset: " + preset + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubEnsurePreset(t *testing.T, err error) *[]string {
	t.Helper()
	var asked []string
	prev := ensurePresetFn
	ensurePresetFn = func(name string) error {
		asked = append(asked, name)
		return err
	}
	t.Cleanup(func() { ensurePresetFn = prev })
	return &asked
}

// A service's definition is fetched once, when it is installed, so a store
// change after that never reaches a host already running it. The sweep asks for
// every installed preset, and asks for each one once however many services
// share it.
func TestRefreshInstalledPresets_AsksForEachPresetOnce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeInstalledService(t, "solr", "solr")
	writeInstalledService(t, "solr-archive", "solr")
	writeInstalledService(t, "gotenberg", "gotenberg")
	asked := stubEnsurePreset(t, nil)

	if n := RefreshInstalledPresets(); n != 2 {
		t.Errorf("refreshed %d presets, want 2", n)
	}
	want := []string{"gotenberg", "solr"}
	got := append([]string(nil), *asked...)
	sortStrings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("asked for %v, want %v", got, want)
	}
}

// A service defined by hand carries no preset, so there is nothing to refresh
// and nothing to ask the store for.
func TestRefreshInstalledPresets_IgnoresAServiceWithNoPreset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeInstalledService(t, "homemade", "")
	asked := stubEnsurePreset(t, nil)

	if n := RefreshInstalledPresets(); n != 0 {
		t.Errorf("refreshed %d presets, want 0", n)
	}
	if len(*asked) != 0 {
		t.Errorf("asked for %v, want nothing", *asked)
	}
}

// The sweep is a refresh of something that already works, so a store that
// cannot be reached leaves every service exactly as it was.
func TestRefreshInstalledPresets_SurvivesAnUnreachableStore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeInstalledService(t, "solr", "solr")
	stubEnsurePreset(t, os.ErrDeadlineExceeded)

	if n := RefreshInstalledPresets(); n != 0 {
		t.Errorf("refreshed %d presets, want 0", n)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
