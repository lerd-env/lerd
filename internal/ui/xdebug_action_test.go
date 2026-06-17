package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

type noopXdebugLifecycle struct{}

func (noopXdebugLifecycle) Start(string) error                { return nil }
func (noopXdebugLifecycle) Stop(string) error                 { return nil }
func (noopXdebugLifecycle) Restart(string) error              { return nil }
func (noopXdebugLifecycle) UnitStatus(string) (string, error) { return "active", nil }
func (noopXdebugLifecycle) AllUnitStates() map[string]string  { return map[string]string{} }

// Toggling Xdebug on from the dashboard must keep a CLI-set on-demand mode
// (start_with_request=trigger) rather than resetting it to connect-on-request.
func TestHandleXdebugAction_PreservesOnDemandStart(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	origLC := podman.UnitLifecycle
	podman.UnitLifecycle = noopXdebugLifecycle{}
	t.Cleanup(func() { podman.UnitLifecycle = origLC })

	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetXdebugMode("8.4", "debug")
	cfg.SetXdebugStart("8.4", "trigger")
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/xdebug/8.4/on?mode=debug", nil)
	req.RemoteAddr = "127.0.0.1:5050"
	rec := httptest.NewRecorder()
	handleXdebugAction(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if resp["ok"] != true {
		t.Fatalf("ok = %v, body=%s", resp["ok"], rec.Body.String())
	}

	got, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if start := got.GetXdebugStart("8.4"); start != "trigger" {
		t.Errorf("xdebug_start = %q, want trigger preserved through the dashboard toggle", start)
	}
}
