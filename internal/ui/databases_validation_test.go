package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const traversalDB = "../../../../../../home/george/Code"

// decodeDBAction reads the {ok,error} envelope the mutating endpoints return.
func decodeDBAction(t *testing.T, rec *httptest.ResponseRecorder) dbActionResponse {
	t.Helper()
	var got dbActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return got
}

// The JSON-body handlers must reject a traversing database name before it
// reaches serviceops, where it would be joined straight into a snapshot path.
func TestDatabaseBodyHandlersRejectTraversal(t *testing.T) {
	cases := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, string)
		body    string
	}{
		{"snapshot delete", handleSnapshotDelete, `{"database":"` + traversalDB + `","name":"src"}`},
		{"snapshot restore", handleSnapshotRestore, `{"database":"` + traversalDB + `","name":"src"}`},
		{"snapshot create", handleSnapshotCreate, `{"database":"` + traversalDB + `"}`},
		{"drop", handleDatabaseDrop, `{"name":"a'b"}`},
		{"create", handleDatabaseCreate, `{"name":"a` + "`" + `b"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/databases/postgres/x", strings.NewReader(tc.body))
			tc.handler(rec, req, "postgres")

			got := decodeDBAction(t, rec)
			if got.OK {
				t.Fatal("handler accepted an invalid database name")
			}
			if !strings.Contains(got.Error, "invalid database name") {
				t.Errorf("error = %q, want a rejection naming the invalid database", got.Error)
			}
		})
	}
}

// The export endpoints read straight off disk without the engine running, so
// they must reject before opening a path.
func TestDatabaseExportHandlersRejectTraversal(t *testing.T) {
	cases := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, string)
		query   string
	}{
		{"database export", handleDatabaseExport, "?database=" + traversalDB},
		{"snapshot export", handleSnapshotExport, "?database=" + traversalDB + "&name=src"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/databases/postgres/export"+tc.query, nil)
			tc.handler(rec, req, "postgres")

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rec.Body.String(), "invalid database name") {
				t.Errorf("body = %q, want a rejection naming the invalid database", rec.Body.String())
			}
		})
	}
}

// An empty database name still reports the missing-value message rather than
// the pattern rejection, so the browser copy does not regress.
func TestDatabaseHandlersRejectEmptyName(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/postgres/create", strings.NewReader(`{"name":""}`))
	handleDatabaseCreate(rec, req, "postgres")

	got := decodeDBAction(t, rec)
	if got.OK || !strings.Contains(got.Error, "required") {
		t.Errorf("error = %q, want a required-field message", got.Error)
	}
}
