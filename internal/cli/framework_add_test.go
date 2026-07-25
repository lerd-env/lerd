package cli

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

// storeServer stands up a fake store publishing a single framework at one
// version, so an install can be driven without the network.
func storeServer(t *testing.T, name, label, version string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(store.Index{
			Frameworks: []store.IndexEntry{{
				Name:     name,
				Label:    label,
				Versions: []string{version},
				Latest:   version,
			}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/"+name+"/"+version+".yaml", func(w http.ResponseWriter, _ *http.Request) {
		body := "name: " + name + "\nlabel: " + label + "\nversion: \"" + version + "\"\npublic_dir: public\n"
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A bare name the store publishes is installed from the store, no --public-dir
// required — the whole point of issue #1163.
func TestAddFrameworkFromStore_InstallsPublishedName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	srv := storeServer(t, "cakephp", "CakePHP", "5")
	client := &store.Client{BaseURL: srv.URL}

	if err := addFrameworkFromStore(client, "cakephp"); err != nil {
		t.Fatalf("addFrameworkFromStore: %v", err)
	}

	path := filepath.Join(config.StoreFrameworksDir(), "cakephp@5.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s written: %v", path, err)
	}
	if !strings.Contains(string(data), "label: CakePHP") {
		t.Errorf("installed definition missing label:\n%s", data)
	}
}

// An explicit name@version installs that version rather than the latest.
func TestAddFrameworkFromStore_ExplicitVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(store.Index{
			Frameworks: []store.IndexEntry{{
				Name: "symfony", Label: "Symfony", Versions: []string{"8", "7"}, Latest: "8",
			}},
		})
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/symfony/7.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: symfony\nlabel: Symfony\nversion: \"7\"\npublic_dir: public\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &store.Client{BaseURL: srv.URL}
	if err := addFrameworkFromStore(client, "symfony@7"); err != nil {
		t.Fatalf("addFrameworkFromStore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.StoreFrameworksDir(), "symfony@7.yaml")); err != nil {
		t.Errorf("expected symfony@7 installed: %v", err)
	}
}

// A name the store doesn't publish points at hand-authoring instead of the old
// bare "--public-dir is required".
func TestAddFrameworkFromStore_UnknownNamePointsAtAuthoring(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	srv := storeServer(t, "cakephp", "CakePHP", "5")
	client := &store.Client{BaseURL: srv.URL}

	err := addFrameworkFromStore(client, "nonesuch")
	if err == nil {
		t.Fatal("expected an error for a name not in the store")
	}
	if !strings.Contains(err.Error(), "not in the store") || !strings.Contains(err.Error(), "--public-dir") {
		t.Errorf("error should name the store and point at authoring, got: %v", err)
	}
}

// Running the real command with a bare published name routes to the store
// install and never trips the --public-dir gate.
func TestFrameworkAddCmd_BareNameInstallsFromStore(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	srv := storeServer(t, "drupal", "Drupal", "11")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	cmd := newFrameworkAddCmd()
	cmd.SetArgs([]string{"drupal"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add drupal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.StoreFrameworksDir(), "drupal@11.yaml")); err != nil {
		t.Errorf("expected drupal@11 installed from store: %v", err)
	}
}

// Authoring flags still take the hand-authored path even for a name the store
// happens to publish, so --public-dir governs and a user file is written.
func TestFrameworkAddCmd_AuthoringFlagsStayLocal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	srv := storeServer(t, "drupal", "Drupal", "11")
	t.Setenv("LERD_STORE_BASE_URL", srv.URL)

	cmd := newFrameworkAddCmd()
	cmd.SetArgs([]string{"myfw", "--public-dir", "web", "--detect-file", "myfw.php"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add myfw: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.FrameworksDir(), "myfw.yaml")); err != nil {
		t.Errorf("expected hand-authored myfw.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.StoreFrameworksDir(), "myfw@11.yaml")); err == nil {
		t.Error("hand-authored add should not touch the store dir")
	}
}
