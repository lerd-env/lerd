package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// refreshStorePresets must cache the store preset backing every installed
// service so it keeps resolving offline after an upgrade. A service whose preset
// moved out of the embedded bundle (e.g. pgadmin) is exactly the case that
// regresses without this: its config-mount files come from LoadPreset, which
// fails offline unless the preset was cached locally at install/update time.
func TestRefreshStorePresets_CachesInstalledServicePresets(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	// An installed service backed by a store-only preset.
	if err := config.SaveCustomService(&config.CustomService{
		Name:   "pgadmin",
		Image:  "docker.io/dpage/pgadmin4:latest",
		Preset: "pgadmin",
	}); err != nil {
		t.Fatal(err)
	}

	fetched := false
	mux := http.NewServeMux()
	mux.HandleFunc("/pgadmin.yaml", func(w http.ResponseWriter, _ *http.Request) {
		fetched = true
		_, _ = w.Write([]byte("name: pgadmin\nimage: docker.io/dpage/pgadmin4:latest\ndescription: fresh-from-store\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	refreshStorePresets()

	if !fetched {
		t.Fatal("refreshStorePresets did not fetch the installed service's preset")
	}
	cached := filepath.Join(config.StorePresetsDir(), "pgadmin.yaml")
	data, err := os.ReadFile(cached)
	if err != nil {
		t.Fatalf("preset was not cached locally: %v", err)
	}
	if !strings.Contains(string(data), "fresh-from-store") {
		t.Errorf("cached preset is not the fetched body, got:\n%s", data)
	}
}

// A service with no preset (a hand-rolled custom service) has nothing to fetch,
// so the refresh must not reach out to the store for it.
func TestRefreshStorePresets_SkipsPresetlessServices(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := config.SaveCustomService(&config.CustomService{
		Name:  "gotenberg",
		Image: "docker.io/gotenberg/gotenberg:8",
	}); err != nil {
		t.Fatal(err)
	}

	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	refreshStorePresets()

	if reached {
		t.Error("refreshStorePresets fetched from the store for a presetless service")
	}
}
