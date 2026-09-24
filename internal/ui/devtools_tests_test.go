package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

func postDevtoolsTests(t *testing.T, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/devtools/tests", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	handleDevtoolsTests(rec, req)
	return rec.Code
}

func TestHandleDevtoolsTests_PersistsAndSwitchesTheReceiver(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := withDumpsServer(t)
	testEv := dumps.Event{V: 1, ID: "t", Kind: "query", Ctx: dumps.Context{Test: true}}

	if code := postDevtoolsTests(t, `{"enable":true}`); code != http.StatusOK {
		t.Fatalf("enable status = %d", code)
	}
	cfg, _ := config.LoadGlobal()
	if !cfg.IsDevtoolsTests() {
		t.Error("enable was not persisted")
	}
	srv.Push(testEv)
	if srv.Len() != 1 {
		t.Fatalf("test event dropped after enabling; len = %d", srv.Len())
	}
	if !strings.Contains(string(buildDevtoolsStatusJSON()), `"tests":true`) {
		t.Errorf("status = %s, want tests:true", buildDevtoolsStatusJSON())
	}

	if code := postDevtoolsTests(t, `{"enable":false}`); code != http.StatusOK {
		t.Fatalf("disable status = %d", code)
	}
	if srv.Len() != 0 {
		t.Errorf("buffered test event kept after disabling; len = %d", srv.Len())
	}
}

func TestHandleDevtoolsTests_RefusesRemoteSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/devtools/tests", strings.NewReader(`{"enable":true}`))
	rec := httptest.NewRecorder()
	handleDevtoolsTests(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
