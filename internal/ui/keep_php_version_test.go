package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	phpPkg "github.com/geodro/lerd/internal/php"
)

// Starting a worker from the dashboard ran it on the newest PHP composer.json
// allowed instead of the version the site is served on (#2667).
func TestQueueStartUsesThePHPVersionTheSiteIsServedOn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	prevInstalled, prevStart, prevSync := phpPkg.InstalledForRuntimeFn, startFrameworkWorkerFn, syncWorkersLaterFn
	t.Cleanup(func() {
		phpPkg.InstalledForRuntimeFn, startFrameworkWorkerFn, syncWorkersLaterFn = prevInstalled, prevStart, prevSync
	})
	phpPkg.InstalledForRuntimeFn = func() ([]string, error) { return []string{"8.4", "8.5"}, nil }
	started := make(chan string, 1)
	startFrameworkWorkerFn = func(_, _, phpVersion, _ string) error { started <- phpVersion; return nil }
	syncWorkersLaterFn = func(*config.Site) {}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^8.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/shop.test/queue:start", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	handleSiteAction(httptest.NewRecorder(), req)

	select {
	case v := <-started:
		if v != "8.4" {
			t.Errorf("queue worker started on PHP %s, want 8.4", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queue worker was never started")
	}
}
