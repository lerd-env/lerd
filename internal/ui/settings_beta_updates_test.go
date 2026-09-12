package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func postBetaUpdates(t *testing.T, enabled bool) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/beta-updates", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleSettingsBetaUpdates(rec, req)
	return rec
}

func TestBetaUpdatesPersistsAndIsReportedBySettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	isolateLaunchAgents(t)

	if rec := postBetaUpdates(t, true); rec.Code != http.StatusOK {
		t.Fatalf("POST returned %d: %s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		t.Fatalf("loading config: %v", err)
	}
	if !cfg.IsBetaChannel() {
		t.Error("config was not updated")
	}

	rec := httptest.NewRecorder()
	handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var resp SettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if !resp.BetaUpdates {
		t.Error("beta_updates missing from GET /api/settings")
	}
}

// Leaving the beta line has to be as easy as joining it, or the only way back
// to stable is editing the config by hand.
func TestBetaUpdatesTurnsBackOff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	isolateLaunchAgents(t)

	postBetaUpdates(t, true)
	postBetaUpdates(t, false)

	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.IsBetaChannel() {
		t.Error("beta updates stayed on after being turned off")
	}
}
