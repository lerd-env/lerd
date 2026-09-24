package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func postStreaming(t *testing.T, h http.HandlerFunc, enabled bool) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return out
}

func TestStreamingModeIsRefusedUntilTheFeatureIsEnabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if out := postStreaming(t, handleSettingsStreaming, true); out["ok"] != false {
		t.Fatalf("streaming on while disabled: got %v, want refusal", out)
	}
	if cfg, _ := config.LoadGlobal(); cfg.UI.StreamingMode {
		t.Error("mode saved despite the refusal")
	}
}

func TestStreamingFeatureIsReportedInStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if out := postStreaming(t, handleSettingsStreamingEnabled, true); out["ok"] != true {
		t.Fatalf("enable refused: %v", out)
	}
	if out := postStreaming(t, handleSettingsStreaming, true); out["ok"] != true {
		t.Fatalf("streaming on refused once enabled: %v", out)
	}
	if st := buildStatus(); !st.StreamingEnabled || !st.StreamingMode {
		t.Errorf("status = enabled %v mode %v, want both true", st.StreamingEnabled, st.StreamingMode)
	}

	postStreaming(t, handleSettingsStreamingEnabled, false)
	if st := buildStatus(); st.StreamingEnabled || st.StreamingMode {
		t.Errorf("after disabling: enabled %v mode %v, want both false", st.StreamingEnabled, st.StreamingMode)
	}
}
