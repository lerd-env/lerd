package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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

// projectNewMultiVersionStore publishes two majors of one framework, each with a
// create command of its own, so a test can tell which definition was scaffolded
// from rather than trusting the resolution.
func projectNewMultiVersionStore(t *testing.T, name string, versions map[string]string, latest string) *httptest.Server {
	t.Helper()
	list := make([]string, 0, len(versions))
	for v := range versions {
		list = append(list, v)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(list)))
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(store.Index{
			Frameworks: []store.IndexEntry{{Name: name, Label: name, Versions: list, Latest: latest}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	for version, create := range versions {
		body := "name: " + name + "\nlabel: " + name + "\nversion: \"" + version + "\"\npublic_dir: public\ncreate: \"" + create + "\"\n"
		mux.HandleFunc("/"+name+"/"+version+".yaml", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A caller that names a major gets that major's create command, not the newest
// one the store publishes, which is what `lerd new --framework-version` does.
func TestExecProjectNew_ScaffoldsTheRequestedMajor(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := projectNewMultiVersionStore(t, "codeigniter", map[string]string{"4": "touch", "5": "true"}, "5")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	path := filepath.Join(tmp, "proj")
	resp, rpcErr := execProjectNew(map[string]any{
		"path":      path,
		"framework": "codeigniter",
		"version":   "4",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if resultIsError(resp) {
		t.Fatalf("project_new refused a published major: %s", resultText(t, resp))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected major 4's create command to have run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.StoreFrameworksDir(), "codeigniter@4.yaml")); err != nil {
		t.Errorf("expected major 4's definition cached locally: %v", err)
	}
}

// A major the store does not publish is refused by name and version, rather than
// silently scaffolding whatever is current.
func TestExecProjectNew_UnpublishedMajorIsRefused(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	srv := projectNewMultiVersionStore(t, "codeigniter", map[string]string{"4": "touch", "5": "true"}, "5")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	resp, rpcErr := execProjectNew(map[string]any{
		"path":      filepath.Join(tmp, "proj"),
		"framework": "codeigniter",
		"version":   "99",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if !resultIsError(resp) {
		t.Fatalf("expected an unknown-major error, got: %s", resultText(t, resp))
	}
	if text := resultText(t, resp); !strings.Contains(text, "99") {
		t.Errorf("error should name the major that is missing, got: %s", text)
	}
}

// A version with no framework behind it is a mistake the CLI refuses too: the
// default framework would silently absorb a major meant for another one.
func TestExecProjectNew_VersionNeedsAFramework(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	resp, rpcErr := execProjectNew(map[string]any{
		"path":    filepath.Join(tmp, "proj"),
		"version": "11",
	})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if !resultIsError(resp) {
		t.Fatalf("expected a version-without-framework error, got: %s", resultText(t, resp))
	}
}

// A starter with a skeleton of its own is created from that skeleton rather than
// the framework's, so the target only exists when the starter's command ran.
func TestExecProjectNew_StartsFromAStartersSkeleton(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"frameworks":[{"name":"acme","label":"Acme","versions":["2"],"latest":"2"}],
			"starters":[{"name":"kit","label":"Kit","framework":{"name":"acme","min":"2"}}]}`))
	})
	mux.HandleFunc("/acme/2.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: acme\nlabel: Acme\nversion: \"2\"\npublic_dir: public\ncreate: \"true\"\n"))
	})
	mux.HandleFunc("/starters/acme/kit.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("starter: kit\nlabel: Kit\nframework:\n  name: acme\n  min: \"2\"\ncreate: touch\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	path := filepath.Join(tmp, "proj")
	resp, rpcErr := execProjectNew(map[string]any{"path": path, "framework": "acme", "starter": "kit"})
	if rpcErr != nil {
		t.Fatalf("rpc error: %v", rpcErr)
	}
	if resultIsError(resp) {
		t.Fatalf("project_new refused the starter: %s", resultText(t, resp))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected the starter's create command to have run: %v", err)
	}
}

func TestExecProjectNew_StarterNeedsAFramework(t *testing.T) {
	resp, _ := execProjectNew(map[string]any{"path": filepath.Join(t.TempDir(), "p"), "starter": "filament"})
	if !resultIsError(resp) || !strings.Contains(resultText(t, resp), "starter needs a framework") {
		t.Fatalf("expected a starter without a framework to be refused, got %s", resultText(t, resp))
	}
}
