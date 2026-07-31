package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func localPost(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "localhost:7073"
	req.Header.Set("X-Lerd-CSRF", "1")
	return req
}

// Opting managed services in while lerd is loopback only would persist a
// setting that publishes nothing, which reads as exposure that never happened.
func TestServicesOnRefusedWhileLoopbackOnly(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	rec := httptest.NewRecorder()
	handleLANStatus(rec, localPost("/api/lan/status", `{"action":"services_on"}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	cfg, _ := config.LoadGlobal()
	if cfg != nil && cfg.LAN.ServicesExposed {
		t.Fatal("refused request still persisted lan.services_exposed")
	}
}

// Opting out has to keep working, or a setting armed before an unexpose can
// never be cleared from the dashboard.
func TestServicesOffAllowedWhileLoopbackOnly(t *testing.T) {
	setupConfigDirRaw(t, "", "", false)

	rec := httptest.NewRecorder()
	handleLANStatus(rec, localPost("/api/lan/status", `{"action":"services_off"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestFullAccessRefusedWhileLoopbackOnly(t *testing.T) {
	setupConfigDirRaw(t, "alice", "s3cret", false)

	rec := httptest.NewRecorder()
	handleRemoteControl(rec, localPost("/api/remote-control", `{"action":"full-access","enabled":true}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	cfg, _ := config.LoadGlobal()
	if cfg != nil && cfg.UI.RemoteFullAccess {
		t.Fatal("refused request still persisted ui.remote_full_access")
	}
}

func TestFullAccessOffAllowedWhileLoopbackOnly(t *testing.T) {
	setupConfigDirRaw(t, "alice", "s3cret", false)

	rec := httptest.NewRecorder()
	handleRemoteControl(rec, localPost("/api/remote-control", `{"action":"full-access","enabled":false}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}
