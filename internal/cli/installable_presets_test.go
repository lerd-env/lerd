package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The install picker merges the local presets (embedded defaults) with the
// external store index, carrying versions through so multi-version store
// presets still render a version dropdown.
func TestListInstallablePresets_MergesStoreIndex(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"services":[{"name":"store-only-widget","description":"only in the store","versions":[{"tag":"2","label":"2 LTS","image":"example/widget:2"}],"default_version":"2"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("LERD_SERVICES_BASE_URL", srv.URL)

	metas, err := ListInstallablePresets()
	if err != nil {
		t.Fatalf("ListInstallablePresets: %v", err)
	}
	var storeEntry *config.PresetMeta
	names := map[string]bool{}
	for i := range metas {
		names[metas[i].Name] = true
		if metas[i].Name == "store-only-widget" {
			storeEntry = &metas[i]
		}
	}
	if !names["mysql"] {
		t.Error("local default mysql missing from the merged list")
	}
	if storeEntry == nil {
		t.Fatal("store-only preset was not merged into the installable list")
	}
	if len(storeEntry.Versions) != 1 || storeEntry.Versions[0].Tag != "2" {
		t.Errorf("store preset versions not carried through: %+v", storeEntry.Versions)
	}
}

// Offline (store unreachable) must degrade to the local presets, not error out
// and blank the picker.
func TestListInstallablePresets_OfflineReturnsLocal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LERD_SERVICES_BASE_URL", "http://127.0.0.1:1")

	metas, err := ListInstallablePresets()
	if err != nil {
		t.Fatalf("offline must not error: %v", err)
	}
	names := map[string]bool{}
	for _, m := range metas {
		names[m.Name] = true
	}
	if !names["mysql"] {
		t.Error("offline: local default presets must still be listed")
	}
}
