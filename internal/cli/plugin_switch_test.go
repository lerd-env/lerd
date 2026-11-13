package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// serveUISocket stands in for a running lerd-ui on its unix socket, answering
// every request with status and recording the bodies it got.
func serveUISocket(t *testing.T, status int) *[]map[string]any {
	t.Helper()
	dir, err := os.MkdirTemp("", "ui")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "ui.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	got := &[]map[string]any{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["path"] = r.URL.Path
		*got = append(*got, body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("plugin nope has no site tab\n"))
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	orig := uiSocketPath
	uiSocketPath = func() string { return sock }
	t.Cleanup(func() { uiSocketPath = orig })
	return got
}

func failLocal(t *testing.T) func(*config.Site, string, bool) error {
	return func(*config.Site, string, bool) error {
		t.Error("switched locally while lerd-ui is running")
		return nil
	}
}

// A running dashboard makes the switch itself, so pages already open hear of it.
func TestSwitchSitePlugin_GoesThroughARunningUI(t *testing.T) {
	got := serveUISocket(t, http.StatusOK)
	site := &config.Site{Name: "shop"}
	if err := switchSitePlugin(site, "agents", "site_tab", false, failLocal(t)); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0]["path"] != "/api/page-plugins" || (*got)[0]["site"] != "shop" || (*got)[0]["plugin"] != "agents" || (*got)[0]["kind"] != "site_tab" || (*got)[0]["on"] != false {
		t.Errorf("lerd-ui got %v", *got)
	}
}

func TestSwitchSitePlugin_ReportsWhatTheUIRefused(t *testing.T) {
	serveUISocket(t, http.StatusBadRequest)
	err := switchSitePlugin(&config.Site{Name: "shop"}, "nope", "site_tab", false, failLocal(t))
	if err == nil || err.Error() != "plugin nope has no site tab" {
		t.Errorf("err = %v, want lerd-ui's reason", err)
	}
}

func TestSwitchSitePlugin_SwitchesLocallyWithoutAUI(t *testing.T) {
	orig := uiSocketPath
	uiSocketPath = func() string { return filepath.Join(t.TempDir(), "missing.sock") }
	t.Cleanup(func() { uiSocketPath = orig })
	called := false
	set := func(s *config.Site, plugin string, on bool) error {
		called = s.Name == "shop" && plugin == "agents" && !on
		return nil
	}
	if err := switchSitePlugin(&config.Site{Name: "shop"}, "agents", "site_tab", false, set); err != nil || !called {
		t.Errorf("err = %v, local switch called = %v", err, called)
	}
}
