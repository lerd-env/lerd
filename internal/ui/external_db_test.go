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
			rec := httptest.NewRecorder()
			handleExternalDB(rec, httptest.NewRequest(http.MethodPost, "/api/services/external", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest {
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
