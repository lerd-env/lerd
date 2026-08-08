package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/eventbus"
)

func TestBuildProfilerStatusJSON_HasEnabledField(t *testing.T) {
	raw := buildProfilerStatusJSON()
	var got struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v; raw=%s", err, string(raw))
	}
}

func TestAssembleSnapshot_IncludesProfilerStatusField(t *testing.T) {
	payload := []byte(`{"enabled":true}`)
	frame := assembleSnapshot(nil, nil, nil, nil, nil, nil, payload, nil, []string{eventbus.KindProfilerStatus})
	var decoded struct {
		Type           string          `json:"type"`
		ProfilerStatus json.RawMessage `json:"profiler_status"`
	}
	if err := json.Unmarshal(frame, &decoded); err != nil {
		t.Fatalf("decode frame: %v; raw=%s", err, string(frame))
	}
	if decoded.Type != eventbus.KindProfilerStatus {
		t.Errorf("type = %q, want %q", decoded.Type, eventbus.KindProfilerStatus)
	}
	if !bytes.Equal(decoded.ProfilerStatus, payload) {
		t.Errorf("profiler_status = %s, want %s", decoded.ProfilerStatus, payload)
	}
}

// The dashboard fires a request and then waits for its profile to exist before
// opening SPX, which it does by watching this count rise.
func TestHandleProfilerCaptures_CountsWhatTheHostProfiled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))

	count := func(query string) int {
		req := httptest.NewRequest("GET", "/api/profiler/captures?"+query, nil)
		rec := httptest.NewRecorder()
		handleProfilerCaptures(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
		}
		return resp.Count
	}

	if n := count("host=acme.test&route=GET+%2Fusers%2F%3Aid"); n != 0 {
		t.Fatalf("count before any capture = %d, want 0", n)
	}

	data := config.SpxDataDir()
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"exec_ts":2000,"http_method":"GET","http_host":"acme.test","http_request_uri":"/users/9","cli":0}`
	if err := os.WriteFile(filepath.Join(data, "cap.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := count("host=acme.test&route=GET+%2Fusers%2F%3Aid"); n != 1 {
		t.Errorf("count after the capture = %d, want 1", n)
	}
	if n := count("host=acme.test&route=GET+%2Fcheckout"); n != 0 {
		t.Errorf("another route = %d, want 0", n)
	}
	if n := count(""); n != 0 {
		t.Errorf("no host asked for = %d, want 0", n)
	}
}

func TestHandleProfilerCaptures_PostIsRejected(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/profiler/captures", nil)
	rec := httptest.NewRecorder()
	handleProfilerCaptures(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleProfilerClear_RejectsNonLoopback(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/profiler/clear", nil)
	req.RemoteAddr = "192.168.1.50:42000"
	rec := httptest.NewRecorder()
	handleProfilerClear(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleProfilerClear_GetIsRejected(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/profiler/clear", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleProfilerClear(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandleProfilerClear_LoopbackReturnsRemovedCount(t *testing.T) {
	// Point the profiler data dir at a throwaway location so the handler
	// never touches the developer's real captured reports.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))

	req := httptest.NewRequest("POST", "/api/profiler/clear", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleProfilerClear(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Removed int `json:"removed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
}
