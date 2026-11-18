package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func postWorktreeSibling(t *testing.T, enabled bool) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/worktree-sibling", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleSettingsWorktreeSibling(rec, req)
	return rec
}

func getSettings(t *testing.T) SettingsResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var resp SettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func TestWorktreeSiblingPersistsAndTurnsBackOff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	isolateLaunchAgents(t)

	if getSettings(t).WorktreeSibling {
		t.Fatal("a fresh config must default to worktrees inside the project")
	}
	if rec := postWorktreeSibling(t, true); rec.Code != http.StatusOK {
		t.Fatalf("POST returned %d: %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.WorktreeLocation != "sibling" {
		t.Fatalf("config holds %q, %v; want sibling", cfg.WorktreeLocation, err)
	}
	if !getSettings(t).WorktreeSibling {
		t.Error("worktree_sibling missing from GET /api/settings")
	}

	postWorktreeSibling(t, false)
	cfg, _ = config.LoadGlobal()
	if cfg.WorktreeLocation != "" {
		t.Errorf("turning it off left %q in config.yaml", cfg.WorktreeLocation)
	}
}
