package store

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

const starterIndexJSON = `{"frameworks":[],"starters":[
	{"name":"filament","label":"Filament","framework":{"name":"acme","min":"11"}},
	{"name":"impostor","label":"Impostor","framework":{"name":"acme"}}
]}`

func starterServer(t *testing.T, indexHits *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		indexHits.Add(1)
		w.Write([]byte(starterIndexJSON)) //nolint:errcheck
	})
	mux.HandleFunc("/starters/acme/filament.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("starter: filament\nlabel: Filament\nframework:\n  name: acme\n  min: \"11\"\nrequire:\n  - filament/filament\n")) //nolint:errcheck
	})
	mux.HandleFunc("/starters/acme/impostor.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("starter: filament\nlabel: Filament\nframework:\n  name: acme\nrequire:\n  - filament/filament\n")) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	return httptest.NewServer(mux)
}

func TestResolveStarter_fetchesAListedStarterForASupportedMajor(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var hits atomic.Int32
	srv := starterServer(t, &hits)
	defer srv.Close()

	s, err := testClient(t, srv).ResolveStarter("acme", "12", "filament")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Require) != 1 || s.Require[0] != "filament/filament" {
		t.Errorf("Require = %v", s.Require)
	}
	// A machine that had no cached index fetched one, and keeps it.
	if len(config.StoreStarters("acme")) != 2 {
		t.Error("the index fetched to find the starter should have been cached")
	}
}

func TestResolveStarter_refusesAMajorOutsideTheStartersRange(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var hits atomic.Int32
	srv := starterServer(t, &hits)
	defer srv.Close()

	_, err := testClient(t, srv).ResolveStarter("acme", "10", "filament")
	if err == nil || !strings.Contains(err.Error(), "11 and newer") {
		t.Fatalf("expected the supported range in the error, got %v", err)
	}
}

func TestResolveStarter_namesTheStartersAFrameworkHasWhenOneIsUnknown(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var hits atomic.Int32
	srv := starterServer(t, &hits)
	defer srv.Close()

	_, err := testClient(t, srv).ResolveStarter("acme", "12", "nope")
	if err == nil || !strings.Contains(err.Error(), "filament") {
		t.Fatalf("expected the available starters in the error, got %v", err)
	}
}

func TestResolveStarter_refusesAFileNamingAnotherStarter(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var hits atomic.Int32
	srv := starterServer(t, &hits)
	defer srv.Close()

	if _, err := testClient(t, srv).ResolveStarter("acme", "12", "impostor"); err == nil {
		t.Fatal("expected a file declaring another starter to be refused")
	}
}

func TestResolveStarter_refusesANameThatCannotBeAFile(t *testing.T) {
	var hits atomic.Int32
	srv := starterServer(t, &hits)
	defer srv.Close()

	if _, err := testClient(t, srv).ResolveStarter("acme", "12", "../../etc/passwd"); err == nil {
		t.Fatal("expected an invalid starter name to be refused")
	}
	if hits.Load() != 0 {
		t.Error("an invalid name should be refused before reaching the store")
	}
}
