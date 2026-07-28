package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/tools"
)

func toolUpdateResult(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, rec.Body.String())
	}
	return out
}

func TestHandleToolUpdate(t *testing.T) {
	orig := updateToolFn
	t.Cleanup(func() { updateToolFn = orig })

	t.Run("applies the named tool", func(t *testing.T) {
		var asked string
		updateToolFn = func(name string) error { asked = name; return nil }

		rec := httptest.NewRecorder()
		handleTools(rec, httptest.NewRequest(http.MethodPost, "/api/tools/mkcert/update", nil))

		if asked != "mkcert" {
			t.Errorf("updated %q, want mkcert", asked)
		}
		if got := toolUpdateResult(t, rec)["ok"]; got != true {
			t.Errorf("ok = %v, want true", got)
		}
	})

	// A rejected checksum is the reason this is per tool, so the card has to be
	// told which tool failed and why rather than getting a generic error.
	t.Run("carries the failure back", func(t *testing.T) {
		updateToolFn = func(string) error { return errors.New("checksum mismatch for composer") }

		rec := httptest.NewRecorder()
		handleTools(rec, httptest.NewRequest(http.MethodPost, "/api/tools/composer/update", nil))

		out := toolUpdateResult(t, rec)
		if out["ok"] == true {
			t.Error("a failed update reported ok")
		}
		if msg, _ := out["error"].(string); !strings.Contains(msg, "checksum mismatch") {
			t.Errorf("error = %q, want the underlying reason", msg)
		}
	})

	t.Run("refuses a read method", func(t *testing.T) {
		updateToolFn = func(string) error { t.Error("GET reached the installer"); return nil }
		rec := httptest.NewRecorder()
		handleTools(rec, httptest.NewRequest(http.MethodGet, "/api/tools/mkcert/update", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", rec.Code)
		}
	})

	// The pins live in one manifest, so the check is one fetch for every tool
	// rather than one per card, and it answers with the result of that fetch.
	t.Run("re-reads the pins with the cache bypassed", func(t *testing.T) {
		origRefresh, origStatus := refreshPinsFn, toolStatusFn
		t.Cleanup(func() { refreshPinsFn, toolStatusFn = origRefresh, origStatus })
		refreshed := 0
		refreshPinsFn = func(context.Context) *tools.Manifest { refreshed++; return nil }
		toolStatusFn = func(context.Context) []tools.ToolStatus {
			return []tools.ToolStatus{{Name: "composer", Pinned: "2.10.3", Present: true, UpdateAvailable: true}}
		}
		updateToolFn = func(name string) error { t.Errorf("check installed %q", name); return nil }

		rec := httptest.NewRecorder()
		handleTools(rec, httptest.NewRequest(http.MethodPost, "/api/tools/check", nil))

		if refreshed != 1 {
			t.Errorf("refreshed %d times, want 1", refreshed)
		}
		out := toolUpdateResult(t, rec)
		if out["ok"] != true {
			t.Errorf("ok = %v", out["ok"])
		}
		list, _ := out["tools"].([]any)
		if len(list) != 1 {
			t.Fatalf("tools = %v, want the state after the check", out["tools"])
		}
	})

	// The route is a prefix, so every path under it that is not the update
	// action has to miss rather than fall through to an install.
	t.Run("refuses anything that is not the update action", func(t *testing.T) {
		updateToolFn = func(name string) error { t.Errorf("%q reached the installer", name); return nil }
		for _, path := range []string{
			"/api/tools//update",
			"/api/tools/composer",
			"/api/tools/composer/",
			"/api/tools/composer/update/extra",
			"/api/tools/../../etc/update",
		} {
			rec := httptest.NewRecorder()
			handleTools(rec, httptest.NewRequest(http.MethodPost, path, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404", path, rec.Code)
			}
		}
	})
}
