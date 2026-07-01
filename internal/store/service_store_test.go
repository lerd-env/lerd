package store

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func serviceTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"services":[
			{"name":"demo","description":"Demo document store","family":"demo","dashboard":"http://localhost:9",	"depends_on":["mysql"]},
			{"name":"widget","description":"Widget cache"}
		]}`))
	})
	mux.HandleFunc("/demo.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: demo\nimage: example/demo:1\ndescription: Demo document store\nports:\n  - \"1234:1234\"\n"))
	})
	mux.HandleFunc("/bad.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: bad\n")) // no image and no versions => invalid
	})
	return httptest.NewServer(mux)
}

func serviceTestClient(srv *httptest.Server) *Client {
	return &Client{BaseURL: srv.URL}
}

func TestFetchServiceIndex(t *testing.T) {
	srv := serviceTestServer(t)
	defer srv.Close()
	idx, err := serviceTestClient(srv).FetchServiceIndex()
	if err != nil {
		t.Fatalf("FetchServiceIndex: %v", err)
	}
	if len(idx.Services) != 2 || idx.Services[0].Name != "demo" {
		t.Fatalf("unexpected index: %+v", idx)
	}
	if idx.Services[0].Family != "demo" || idx.Services[0].Dashboard == "" {
		t.Errorf("index entry lost metadata: %+v", idx.Services[0])
	}
}

func TestFetchServicePreset_SavesToCacheAndSeamServesIt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srv := serviceTestServer(t)
	defer srv.Close()

	data, err := serviceTestClient(srv).FetchServicePreset("demo")
	if err != nil {
		t.Fatalf("FetchServicePreset: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("FetchServicePreset returned no bytes")
	}
	if _, err := os.Stat(filepath.Join(config.StorePresetsDir(), "demo.yaml")); err != nil {
		t.Errorf("preset not written to store cache: %v", err)
	}
	// The seam must now serve the fetched preset by name.
	if !config.PresetExists("demo") {
		t.Error("PresetExists(demo) false after fetch")
	}
	p, err := config.LoadPreset("demo")
	if err != nil || p.Image != "example/demo:1" {
		t.Errorf("LoadPreset(demo) = %+v, %v", p, err)
	}
}

func TestFetchServicePreset_RejectsInvalid(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srv := serviceTestServer(t)
	defer srv.Close()

	if _, err := serviceTestClient(srv).FetchServicePreset("bad"); err == nil {
		t.Fatal("expected FetchServicePreset to reject an invalid preset")
	}
	if _, err := os.Stat(filepath.Join(config.StorePresetsDir(), "bad.yaml")); err == nil {
		t.Error("invalid preset must not be written to the store cache")
	}
}

// Full production path: the LERD_SERVICES_BASE_URL override flows through
// origin into NewServiceClient, which fetches, validates, saves to the cache
// dir, and the config seam then serves the preset by name.
func TestNewServiceClient_HonorsEnvOverrideEndToEnd(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srv := serviceTestServer(t)
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	if _, err := NewServiceClient().FetchServicePreset("demo"); err != nil {
		t.Fatalf("FetchServicePreset via env override: %v", err)
	}
	if !config.PresetExists("demo") {
		t.Error("seam does not serve the preset fetched through the real client")
	}
}

func TestSearchServices(t *testing.T) {
	srv := serviceTestServer(t)
	defer srv.Close()
	c := serviceTestClient(srv)

	got, err := c.SearchServices("cache") // matches "Widget cache" description
	if err != nil {
		t.Fatalf("SearchServices: %v", err)
	}
	if len(got) != 1 || got[0].Name != "widget" {
		t.Errorf("SearchServices(cache) = %+v, want [widget]", got)
	}
	if all, _ := c.SearchServices(""); len(all) != 2 {
		t.Errorf("empty query should match all, got %d", len(all))
	}
}
