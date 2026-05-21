package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/eventbus"
)

func TestBuildDumpsStatusJSON_HasExpectedShape(t *testing.T) {
	prev := dumpsServer.Load()
	dumpsServer.Store(nil)
	t.Cleanup(func() { dumpsServer.Store(prev) })

	raw := buildDumpsStatusJSON()
	var got struct {
		Enabled     bool   `json:"enabled"`
		Passthrough bool   `json:"passthrough"`
		Listening   bool   `json:"listening"`
		Addr        string `json:"addr"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v; raw=%s", err, string(raw))
	}
	if got.Addr == "" {
		t.Error("addr should never be empty")
	}
}

func TestAssembleSnapshot_IncludesDumpsStatusField(t *testing.T) {
	payload := []byte(`{"enabled":true,"listening":true}`)
	frame := assembleSnapshot(nil, nil, nil, nil, payload, nil, nil, []string{eventbus.KindDumpsStatus})
	var decoded struct {
		Type        string          `json:"type"`
		DumpsStatus json.RawMessage `json:"dumps_status"`
	}
	if err := json.Unmarshal(frame, &decoded); err != nil {
		t.Fatalf("decode frame: %v; raw=%s", err, string(frame))
	}
	if decoded.Type != eventbus.KindDumpsStatus {
		t.Errorf("type = %q, want %q", decoded.Type, eventbus.KindDumpsStatus)
	}
	if !bytes.Equal(decoded.DumpsStatus, payload) {
		t.Errorf("dumps_status = %s, want %s", decoded.DumpsStatus, payload)
	}
}

// subscribeAndCollect waits up to d for any published event and returns its kinds.
func subscribeAndCollect(t *testing.T, d time.Duration, trigger func()) []string {
	t.Helper()
	sub := eventbus.Default.Subscribe()
	t.Cleanup(func() { eventbus.Default.Unsubscribe(sub) })
	trigger()
	select {
	case evt := <-sub.C:
		return evt.Kinds
	case <-time.After(d):
		return nil
	}
}

func TestHandleDumpsNotifyChanged_PublishesKindDumpsStatus(t *testing.T) {
	kinds := subscribeAndCollect(t, time.Second, func() {
		req := httptest.NewRequest("POST", "/api/dumps/notify-changed", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		handleDumpsNotifyChanged(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
		}
	})
	if len(kinds) != 1 || kinds[0] != eventbus.KindDumpsStatus {
		t.Errorf("expected eventbus.KindDumpsStatus published, got %v", kinds)
	}
}

func TestHandleDumpsNotifyChanged_RejectsNonLoopback(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/dumps/notify-changed", nil)
	req.RemoteAddr = "192.168.1.50:42000"
	rec := httptest.NewRecorder()
	handleDumpsNotifyChanged(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleDumpsNotifyChanged_GetIsRejected(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/dumps/notify-changed", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleDumpsNotifyChanged(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
