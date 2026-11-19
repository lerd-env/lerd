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
	return postNginxFrom(t, method, path, "127.0.0.1:5555")
}

func postNginxFrom(t *testing.T, method, path, peer string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = peer
	handleNginxRoutes(rec, req)
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

// A LAN session without full access sees the flag but may not pull and restart
// nginx, matching the hidden button.
func TestNginxUpdate_RefusesRemoteWithoutFullAccess(t *testing.T) {
	calls := stubNginxUpdate(t, true, nil)
	prev := checkNginxFn
	checks := 0
	checkNginxFn = func() (bool, error) { checks++; return false, nil }
	t.Cleanup(func() { checkNginxFn = prev })
	for _, path := range []string{"/api/nginx/update", "/api/nginx/check"} {
		out := postNginxFrom(t, http.MethodPost, path, "192.168.1.50:5555")
		if out["status"] != float64(http.StatusForbidden) {
			t.Errorf("%s from the LAN: got %v, want 403", path, out)
		}
	}
	if *calls != 0 || checks != 0 {
		t.Errorf("update ran %d times, check %d times, want neither", *calls, checks)
	}
}

// The update answer is cached for an hour, so applying the update has to drop
// it or the flag outlives the update.
func TestNginxUpdate_DropsTheCachedAnswer(t *testing.T) {
	stubNginxUpdate(t, true, nil)
	t.Cleanup(func() { updates.dropFor("") })
	cachedUpdate(updates, "lerd-nginx", func() bool { return true })

	postNginx(t, http.MethodPost, "/api/nginx/update")
	if cachedUpdate(updates, "lerd-nginx", func() bool { return false }) {
		t.Error("nginx still reads as behind after the update")
	}
}
