package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"services":[{"name":"pgadmin"}]}`))
	})
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

// The built-in presets ship embedded and were never published to the service
// store, so asking the store for one is a guaranteed 404 that surfaces on every
// update as a failed step. Only presets the store index lists may be fetched.
func TestRefreshStorePresets_SkipsPresetsTheStoreDoesNotPublish(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	for _, svc := range []*config.CustomService{
		{Name: "postgres", Image: "docker.io/library/postgres:18", Preset: "postgres"},
		{Name: "pgadmin", Image: "docker.io/dpage/pgadmin4:latest", Preset: "pgadmin"},
	} {
		if err := config.SaveCustomService(svc); err != nil {
			t.Fatal(err)
		}
	}

	var asked []string
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"services":[{"name":"pgadmin"}]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, strings.TrimPrefix(r.URL.Path, "/"))
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/pgadmin.yaml", func(w http.ResponseWriter, _ *http.Request) {
		asked = append(asked, "pgadmin.yaml")
		_, _ = w.Write([]byte("name: pgadmin\nimage: docker.io/dpage/pgadmin4:latest\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	refreshStorePresets()

	for _, path := range asked {
		if path == "postgres.yaml" {
			t.Error("refreshStorePresets asked the store for a preset it does not publish")
		}
	}
	if len(asked) == 0 || asked[0] != "pgadmin.yaml" {
		t.Errorf("expected the published preset to be fetched, got %v", asked)
	}
}

// With the store index unreachable there is no way to tell a published preset
// from a built-in, so the refresh does nothing rather than guess and fail. The
// cached and embedded copies keep serving.
func TestRefreshStorePresets_SkipsRefreshWhenTheIndexIsUnreachable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := config.SaveCustomService(&config.CustomService{
		Name:   "pgadmin",
		Image:  "docker.io/dpage/pgadmin4:latest",
		Preset: "pgadmin",
	}); err != nil {
		t.Fatal(err)
	}

	presetAsked := false
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/pgadmin.yaml", func(w http.ResponseWriter, _ *http.Request) {
		presetAsked = true
		_, _ = w.Write([]byte("name: pgadmin\nimage: docker.io/dpage/pgadmin4:latest\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	refreshStorePresets()

	if presetAsked {
		t.Error("refreshStorePresets fetched presets without an index to check them against")
	}
}

// An install with a handful of services pays one round trip per preset, and the
// preset refresh is the same shape as the definition one: overlap the fetches.
func TestRefreshStorePresets_FetchesInParallel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	names := []string{"mysql", "postgres", "redis", "meilisearch"}
	for _, name := range names {
		if err := config.SaveCustomService(&config.CustomService{
			Name: name, Image: "docker.io/library/" + name + ":latest", Preset: name,
		}); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	inFlight, peak := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"services":[{"name":"mysql"},{"name":"postgres"},{"name":"redis"},{"name":"meilisearch"}]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(25 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		name := strings.TrimSuffix(filepath.Base(r.URL.Path), ".yaml")
		_, _ = w.Write([]byte("name: " + name + "\nimage: docker.io/library/" + name + ":latest\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	refreshStorePresets()

	mu.Lock()
	defer mu.Unlock()
	if peak < 2 {
		t.Errorf("peak concurrent fetches = %d, want the refresh to overlap them", peak)
	}
}
