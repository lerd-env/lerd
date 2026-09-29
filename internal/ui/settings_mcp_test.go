package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubMCP swaps the registration hooks so no test touches the real user-scope
// config of any AI assistant on the machine running it.
func stubMCP(t *testing.T, configured bool, enableErr error) (enabled, disabled *int) {
	t.Helper()
	enabled, disabled = new(int), new(int)
	prevEnable, prevDisable, prevConfigured := mcpEnable, mcpDisable, mcpConfigured
	mcpEnable = func() error { *enabled++; return enableErr }
	mcpDisable = func() error { *disabled++; return nil }
	mcpConfigured = func() bool { return configured }
	t.Cleanup(func() { mcpEnable, mcpDisable, mcpConfigured = prevEnable, prevDisable, prevConfigured })
	return enabled, disabled
}

func postMCP(t *testing.T, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/mcp", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	handleSettingsMCP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec, resp
}

func TestMCPSwitchRegistersAndUnregisters(t *testing.T) {
	enabled, disabled := stubMCP(t, false, nil)

	if _, resp := postMCP(t, `{"enabled":true}`); resp["ok"] != true || resp["mcp_global"] != true {
		t.Fatalf("enabling answered %v", resp)
	}
	if *enabled != 1 || *disabled != 0 {
		t.Fatalf("enable ran %d times, disable %d", *enabled, *disabled)
	}

	if _, resp := postMCP(t, `{"enabled":false}`); resp["ok"] != true || resp["mcp_global"] != false {
		t.Fatalf("disabling answered %v", resp)
	}
	if *disabled != 1 {
		t.Fatalf("disable ran %d times", *disabled)
	}
}

func TestMCPSwitchReportsAFailedRegistration(t *testing.T) {
	stubMCP(t, false, errors.New("no home directory"))

	_, resp := postMCP(t, `{"enabled":true}`)
	if resp["ok"] != false || resp["error"] != "no home directory" {
		t.Fatalf("a failed registration answered %v", resp)
	}
}

func TestSettingsReportsWhetherMCPIsRegistered(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	isolateLaunchAgents(t)

	for _, configured := range []bool{false, true} {
		stubMCP(t, configured, nil)
		rec := httptest.NewRecorder()
		handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
		var resp SettingsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
		if resp.MCPGlobal != configured {
			t.Errorf("mcp_global = %v while registered = %v", resp.MCPGlobal, configured)
		}
	}
}

func TestMCPSwitchRejectsNonPOST(t *testing.T) {
	rec := httptest.NewRecorder()
	handleSettingsMCP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/mcp", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET should be rejected, got %d", rec.Code)
	}
}

func TestMCPSwitchRejectsAnInvalidBody(t *testing.T) {
	stubMCP(t, false, nil)
	if rec, _ := postMCP(t, "not json"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid body should be rejected, got %d", rec.Code)
	}
}
