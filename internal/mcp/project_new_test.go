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

// projectNewStore stands up a fake store publishing one framework whose create
// command is a harmless no-op, so project_new can resolve and "scaffold" without
// touching the network or the host toolchain.
func projectNewStore(t *testing.T, name, version string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(store.Index{
			Frameworks: []store.IndexEntry{{
				Name: name, Label: name, Versions: []string{version}, Latest: version,
			}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/"+name+"/"+version+".yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: " + name + "\nlabel: " + name + "\nversion: \"" + version + "\"\npublic_dir: public\ncreate: \"true\"\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// project_new must scaffold a framework the store publishes but the machine has
// not installed, the way linking fetches one on demand, rather than refusing it
// as unknown.
func TestExecProjectNew_FetchesUninstalledFramework(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := projectNewStore(t, "codeigniter", "4")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	resp, rpcErr := execProjectNew(map[string]any{
		"path":      filepath.Join(tmp, "proj"),
		"framework": "codeigniter",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if resultIsError(resp) {
		t.Fatalf("project_new refused an uninstalled published framework: %s", resultText(t, resp))
	}
	if text := resultText(t, resp); !strings.Contains(text, "Project created") {
		t.Errorf("expected a created-project result, got: %s", text)
	}
	if _, err := os.Stat(filepath.Join(config.StoreFrameworksDir(), "codeigniter@4.yaml")); err != nil {
		t.Errorf("expected the fetched definition saved locally: %v", err)
	}
}

// A name the store does not publish keeps the original unknown-framework error.
func TestExecProjectNew_UnknownStaysUnknown(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := projectNewStore(t, "codeigniter", "4")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	resp, rpcErr := execProjectNew(map[string]any{
		"path":      filepath.Join(tmp, "proj"),
		"framework": "nonesuch",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if !resultIsError(resp) {
		t.Fatalf("expected an unknown-framework error, got: %s", resultText(t, resp))
	}
	if text := resultText(t, resp); !strings.Contains(text, "unknown framework") {
		t.Errorf("error should report an unknown framework, got: %s", text)
	}
}
