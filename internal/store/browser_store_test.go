package store

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestFetchBrowserPreset_cachesItAndRefusesAnImpostor(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/browser/inertia.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("name: inertia\nlabel: Inertia\nevents:\n  - event: inertia:invalid\n")) //nolint:errcheck
	})
	mux.HandleFunc("/browser/turbo.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("name: inertia\n")) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := testClient(t, srv)

	p, err := c.FetchBrowserPreset("inertia")
	if err != nil || p.Label != "Inertia" || len(p.Events) != 1 {
		t.Fatalf("FetchBrowserPreset = %+v, %v", p, err)
	}
	if _, err := os.Stat(config.StoreBrowserPresetFile("inertia")); err != nil {
		t.Fatalf("preset not cached: %v", err)
	}
	if _, err := c.FetchBrowserPreset("turbo"); err == nil {
		t.Fatal("a file naming another preset was accepted")
	}
	if _, err := c.FetchBrowserPreset("../etc"); err == nil {
		t.Fatal("a non-slug name was fetched")
	}
}
