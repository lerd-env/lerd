package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleExternalDB_RefusesAnIncompleteForm(t *testing.T) {
	for name, body := range map[string]string{
		"no host":    `{"name":"x","engine":"mysql","port":3306,"user":"me"}`,
		"no user":    `{"name":"x","engine":"mysql","host":"h","port":3306}`,
		"bad port":   `{"name":"x","engine":"mysql","host":"h","port":70000,"user":"me"}`,
		"no name":    `{"engine":"mysql","host":"h","port":3306,"user":"me"}`,
		"not a form": `nope`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/services/external", strings.NewReader(body))
			req.RemoteAddr = "127.0.0.1:5000"
			rec := httptest.NewRecorder()
			handleExternalDB(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("code = %d, body %s", rec.Code, rec.Body)
			}
		})
	}
}

// Saving a server writes credentials and rewrites site env files on the host.
func TestHandleExternalDB_RefusesARemoteSessionWithoutFullAccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, path := range []string{"/api/services/external", "/api/services/external/test", "/api/services/external/update"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"name":"x","engine":"mysql","host":"h","port":3306,"user":"me"}`))
			req.RemoteAddr = "8.8.8.8:5000"
			rec := httptest.NewRecorder()
			handleExternalDB(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("code = %d, body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestHandleExternalDB_OnlyPosts(t *testing.T) {
	rec := httptest.NewRecorder()
	handleExternalDB(rec, httptest.NewRequest(http.MethodGet, "/api/services/external", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("code = %d", rec.Code)
	}
}
