package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/store"
)

// storeFrameworkServer serves a two-framework catalogue with two majors each,
// and returns a snapshot of which paths were asked for. The refresh fetches in
// parallel, so the recording is locked.
func storeFrameworkServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()

	const index = `{"frameworks":[
	  {"name":"drupal","label":"Drupal","versions":["11","10"],"latest":"11","detect":[{"composer":"drupal/core"}]},
	  {"name":"laravel","label":"Laravel","versions":["13","12"],"latest":"13","detect":[{"file":"artisan"}]}
	]}`

	var mu sync.Mutex
	var asked []string
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(index))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name, version := filepath.Base(filepath.Dir(r.URL.Path)), filepath.Base(r.URL.Path)
		mu.Lock()
		asked = append(asked, r.URL.Path)
		mu.Unlock()
		_, _ = fmt.Fprintf(w, "name: %s\nversion: %q\nlabel: %s\npublic_dir: public\n",
			name, version[:len(version)-len(".yaml")], name)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), asked...)
	}
}

func installedDefinitionNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(config.StoreFrameworksDir())
	if err != nil {
		t.Fatalf("reading store dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != "index.json" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// A just-installed machine has nothing cached, so a refresh that only revisits
// what is already on disk fetches nothing at all and leaves the machine with the
// built-ins alone. Every definition the catalogue publishes has to land.
func TestRefreshStoreFrameworks_FetchesTheWholeCatalogueOnAFreshMachine(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv, _ := storeFrameworkServer(t)
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	refreshStoreFrameworks(nil)

	got := installedDefinitionNames(t)
	want := []string{"drupal@10.yaml", "drupal@11.yaml", "laravel@12.yaml", "laravel@13.yaml"}
	if len(got) != len(want) {
		t.Fatalf("installed definitions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("installed definitions = %v, want %v", got, want)
		}
	}
}

// A definition on disk that the catalogue no longer lists must still be
// refreshed. Dropping it from the target list would strand a project whose
// framework was unpublished on a copy that never updates again.
func TestRefreshStoreFrameworks_KeepsRefreshingAnUnpublishedDefinition(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := config.SaveStoreFramework(&config.Framework{
		Name: "statamic", Label: "Statamic", Version: "6", PublicDir: "public",
	}); err != nil {
		t.Fatal(err)
	}

	srv, asked := storeFrameworkServer(t)
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	refreshStoreFrameworks(nil)

	definitions := yamlAsks(asked())
	found := false
	for _, path := range definitions {
		if path == "/statamic/6.yaml" {
			found = true
		}
	}
	if !found {
		t.Errorf("unpublished definition was not refreshed, asked for %v", asked())
	}
	if len(definitions) != 5 {
		t.Errorf("asked for %d definitions, want the catalogue's 4 plus statamic: %v", len(definitions), definitions)
	}
}

// The index the caller already fetched is used as given, so install does not pay
// for a second round trip to the file it just wrote. Nothing is cached on disk
// here, so a target list built from the cache alone would come out empty.
func TestRefreshStoreFrameworks_UsesTheCallersIndex(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv, asked := storeFrameworkServer(t)
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	refreshStoreFrameworks(&store.Index{Frameworks: []store.IndexEntry{
		{Name: "tempest", Label: "Tempest", Versions: []string{"3"}, Latest: "3"},
	}})

	if definitions := yamlAsks(asked()); len(definitions) != 1 || definitions[0] != "/tempest/3.yaml" {
		t.Errorf("asked for %v, want just /tempest/3.yaml", definitions)
	}
}

// yamlAsks keeps only the definition requests. A refresh also asks for each
// framework's mark, which these tests are not about: what they pin is how many
// definitions get pulled and off which index.
func yamlAsks(asked []string) []string {
	var out []string
	for _, path := range asked {
		if strings.HasSuffix(path, ".yaml") {
			out = append(out, path)
		}
	}
	return out
}

// The store publishes dozens of definitions and each fetch is almost all round
// trip, so pulling them one after another is minutes of dead wall clock. The
// refresh has to keep several in flight.
func TestRefreshStoreFrameworks_FetchesInParallel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	var mu sync.Mutex
	inFlight, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(25 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		_, _ = fmt.Fprintf(w, "name: %s\nlabel: %s\npublic_dir: public\n",
			filepath.Base(filepath.Dir(r.URL.Path)), filepath.Base(r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	refreshStoreFrameworks(&store.Index{Frameworks: []store.IndexEntry{
		{Name: "drupal", Versions: []string{"10", "11"}, Latest: "11"},
		{Name: "laravel", Versions: []string{"12", "13"}, Latest: "13"},
	}})

	mu.Lock()
	defer mu.Unlock()
	if peak < 2 {
		t.Errorf("peak concurrent fetches = %d, want the refresh to overlap them", peak)
	}
}
