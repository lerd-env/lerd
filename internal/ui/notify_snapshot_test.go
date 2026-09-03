package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// loopbackPost is the shape the watcher sends: the endpoint is loopback-only,
// and httptest's default peer is a public address.
func loopbackPost(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/internal/snapshot-run", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

// A request from off the machine is refused whatever it carries.
func TestHandleInternalSnapshotNotify_rejectsRemote(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/snapshot-run", strings.NewReader(`{"databases":2,"sites":1}`))
	handleInternalSnapshotNotify(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestSnapshotRunNotification(t *testing.T) {
	n := snapshotRunNotification(3, 2)
	if n.Kind != "snapshot" {
		t.Errorf("kind = %q, want its own category so it can be muted alone", n.Kind)
	}
	if n.Body != "3 databases on 2 sites." {
		t.Errorf("body = %q", n.Body)
	}
	// One of each reads as one of each.
	if got := snapshotRunNotification(1, 1).Body; got != "1 database on 1 site." {
		t.Errorf("singular body = %q", got)
	}
	if n.Params["databases"] != "3" || n.Params["sites"] != "2" {
		t.Errorf("params = %v, want the counts for the localised body", n.Params)
	}
}

// A run that took nothing must not reach the notifier at all, so the endpoint
// refuses a zero count rather than announcing an empty run.
func TestHandleInternalSnapshotNotify(t *testing.T) {
	rec := httptest.NewRecorder()
	req := loopbackPost(`{"databases":0,"sites":0}`)
	handleInternalSnapshotNotify(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty run: status = %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/internal/snapshot-run", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	handleInternalSnapshotNotify(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("GET: status = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = loopbackPost(`{"databases":2,"sites":1}`)
	handleInternalSnapshotNotify(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
