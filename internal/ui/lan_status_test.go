package ui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestLANStatusIncludesManagedServiceExposure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.LAN.Exposed = true
	cfg.LAN.ServicesExposed = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lan/status", nil)
	rec := httptest.NewRecorder()
	handleLANStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Exposed           bool `json:"exposed"`
		ServicesEnabled   bool `json:"services_enabled"`
		ServicesReachable bool `json:"services_reachable"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Exposed || !body.ServicesEnabled || !body.ServicesReachable {
		t.Fatalf("response = %+v, want enabled and reachable services", body)
	}
}

func TestLANStatusCanToggleManagedServiceExposureFromLoopback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveGlobal(&config.GlobalConfig{}); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/lan/status", strings.NewReader(`{"action":"services_on"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "localhost:7073"
	rec := httptest.NewRecorder()
	handleLANStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if !cfg.LAN.ServicesExposed {
		t.Fatal("services_on did not persist managed-service exposure")
	}

	var final struct {
		Result            string `json:"result"`
		ServicesEnabled   bool   `json:"services_enabled"`
		ServicesReachable bool   `json:"services_reachable"`
	}
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		var event struct {
			Result            string `json:"result"`
			ServicesEnabled   bool   `json:"services_enabled"`
			ServicesReachable bool   `json:"services_reachable"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil && event.Result != "" {
			final = event
		}
	}
	if final.Result != "ok" || !final.ServicesEnabled {
		t.Fatalf("final event = %+v", final)
	}
	if final.ServicesReachable {
		t.Fatalf("services must remain unreachable while LAN exposure is off: %+v", final)
	}
}

func TestLANStatusRejectsUnauthenticatedRemoteManagedServiceToggle(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/lan/status", strings.NewReader(`{"action":"services_on"}`))
	req.RemoteAddr = "192.0.2.10:12345"
	rec := httptest.NewRecorder()
	handleLANStatus(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestLANStatusRejectsUnauthenticatedLoopbackReverseProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/lan/status", strings.NewReader(`{"action":"services_on"}`))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "robotbox.example.net"
	rec := httptest.NewRecorder()
	handleLANStatus(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAccessModeRejectsUnauthenticatedLoopbackReverseProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/access-mode", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "robotbox.example.net"
	rec := httptest.NewRecorder()
	handleAccessMode(rec, req)

	var body struct {
		LocalControl bool `json:"local_control"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.LocalControl {
		t.Fatalf("response = %+v, reverse proxy must not grant local control", body)
	}
}
