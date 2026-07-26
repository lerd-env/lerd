package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/store"
)

// storeServerFor stands up a fake store publishing one framework at one version.
func storeServerFor(t *testing.T, name, label, version string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(store.Index{
			Frameworks: []store.IndexEntry{{
				Name: name, Label: label, Versions: []string{version}, Latest: version,
			}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/"+name+"/"+version+".yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: " + name + "\nlabel: " + label + "\nversion: \"" + version + "\"\npublic_dir: public\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A bare name the store publishes installs from the store instead of writing a
// hollow user stub that would shadow it.
func TestExecFrameworkAdd_InstallsPublishedName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := storeServerFor(t, "cakephp", "CakePHP", "5")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)
	defaultSitePath = ""

	resp, rpcErr := execFrameworkAdd(map[string]any{"name": "cakephp"})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if resultIsError(resp) {
		t.Fatalf("unexpected error result: %s", resultText(t, resp))
	}
	if text := resultText(t, resp); !strings.Contains(text, "from the store") {
		t.Errorf("result should confirm a store install, got: %s", text)
	}
	if _, err := os.Stat(filepath.Join(config.StoreFrameworksDir(), "cakephp@5.yaml")); err != nil {
		t.Errorf("expected cakephp@5 installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.FrameworksDir(), "cakephp.yaml")); err == nil {
		t.Error("a store install should not write a user-defined stub")
	}
}

// A name the store doesn't publish points at search and hand-authoring rather
// than silently creating a stub.
func TestExecFrameworkAdd_UnknownNamePointsAtAuthoring(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := storeServerFor(t, "cakephp", "CakePHP", "5")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)
	defaultSitePath = ""

	resp, rpcErr := execFrameworkAdd(map[string]any{"name": "nonesuch"})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if !resultIsError(resp) {
		t.Fatalf("expected an error result for a name not in the store, got: %s", resultText(t, resp))
	}
	if text := resultText(t, resp); !strings.Contains(text, "not in the store") || !strings.Contains(text, "public_dir") {
		t.Errorf("error should name the store and point at authoring, got: %s", text)
	}
}

// Authoring fields still author a user-defined framework by hand.
func TestExecFrameworkAdd_AuthoringFieldsStayLocal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := storeServerFor(t, "cakephp", "CakePHP", "5")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)
	defaultSitePath = ""

	resp, rpcErr := execFrameworkAdd(map[string]any{
		"name":         "myfw",
		"public_dir":   "web",
		"detect_files": []any{"myfw.php"},
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if resultIsError(resp) {
		t.Fatalf("unexpected error result: %s", resultText(t, resp))
	}
	if _, err := os.Stat(filepath.Join(config.FrameworksDir(), "myfw.yaml")); err != nil {
		t.Errorf("expected hand-authored myfw.yaml: %v", err)
	}
}
