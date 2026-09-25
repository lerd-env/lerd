package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func postSetup(t *testing.T, state string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"state": state})
	rec := httptest.NewRecorder()
	handleSettingsSetup(rec, httptest.NewRequest(http.MethodPost, "/api/settings/setup", bytes.NewReader(body)))
	return rec.Body.String()
}

func TestHandleSettingsSetupRecordsTheState(t *testing.T) {
	isolateThemesDir(t)

	for _, want := range []string{"active", "done"} {
		if got := postSetup(t, want); !strings.Contains(got, `"ok":true`) {
			t.Fatalf("POST %q: %s", want, got)
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.UI.Setup != want {
			t.Fatalf("cfg.UI.Setup = %q, want %q", cfg.UI.Setup, want)
		}
	}
}

func TestHandleSettingsSetupRefusesAnUnknownState(t *testing.T) {
	isolateThemesDir(t)

	got := postSetup(t, "finished")
	if !strings.Contains(got, `"ok":false`) || !strings.Contains(got, "finished") {
		t.Fatalf("expected a refusal naming the value, got %s", got)
	}
	cfg, _ := config.LoadGlobal()
	if cfg.UI.Setup != "" {
		t.Errorf("cfg.UI.Setup = %q, want empty", cfg.UI.Setup)
	}
}
