package ui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func stubNginxUpdate(t *testing.T, updated bool, err error) *int {
	t.Helper()
	calls := 0
	prev := updateNginxFn
	updateNginxFn = func(io.Writer) (bool, error) { calls++; return updated, err }
	t.Cleanup(func() { updateNginxFn = prev })
	return &calls
}

func postNginx(t *testing.T, method, path string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	handleNginxRoutes(rec, httptest.NewRequest(method, path, nil))
	if rec.Code != http.StatusOK {
		return map[string]any{"status": float64(rec.Code)}
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestNginxUpdate_Applies(t *testing.T) {
	calls := stubNginxUpdate(t, true, nil)
	out := postNginx(t, http.MethodPost, "/api/nginx/update")
	if *calls != 1 || out["ok"] != true || out["updated"] != true {
		t.Errorf("calls=%d out=%v, want one call and ok+updated", *calls, out)
	}
}

func TestNginxUpdate_ReportsFailure(t *testing.T) {
	stubNginxUpdate(t, false, errors.New("pull refused"))
	out := postNginx(t, http.MethodPost, "/api/nginx/update")
	if out["ok"] != false || out["error"] != "pull refused" {
		t.Errorf("out=%v, want ok=false with the error", out)
	}
}

func TestNginxUpdate_RefusesGet(t *testing.T) {
	calls := stubNginxUpdate(t, true, nil)
	out := postNginx(t, http.MethodGet, "/api/nginx/update")
	if *calls != 0 || out["status"] != float64(http.StatusMethodNotAllowed) {
		t.Errorf("calls=%d out=%v, want 405 and no update", *calls, out)
	}
}

func TestNginxCheck_OfflineIsNotUpToDate(t *testing.T) {
	prev := checkNginxFn
	checkNginxFn = func() (bool, error) { return false, errors.New("registry unreachable") }
	t.Cleanup(func() { checkNginxFn = prev })
	out := postNginx(t, http.MethodPost, "/api/nginx/check")
	if out["ok"] != false {
		t.Errorf("out=%v, want ok=false so the UI says the check failed", out)
	}
}
