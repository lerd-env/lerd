package ui

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postImport(t *testing.T, contentType string, body *bytes.Buffer) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/databases/mysql/import", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	handleDatabaseImport(rec, req, "mysql")
	return rec.Body.String()
}

// The dump is streamed into the engine rather than parsed into a form, so the
// database field has to arrive before the file part it names. A body in the
// wrong order says so instead of importing into an empty name.
func TestDatabaseImportRejectsFileBeforeDatabase(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "havenly.sql")
	if err != nil {
		t.Fatalf("creating file part: %v", err)
	}
	if _, err := part.Write([]byte("SELECT 1;\n")); err != nil {
		t.Fatalf("writing dump: %v", err)
	}
	if err := w.WriteField("database", "havenly"); err != nil {
		t.Fatalf("writing field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}
	got := postImport(t, w.FormDataContentType(), &body)
	if strings.Contains(got, `"ok":true`) {
		t.Fatalf("file-first upload was accepted: %s", got)
	}
}

func TestDatabaseImportRejectsBodyWithoutAFile(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("database", "havenly"); err != nil {
		t.Fatalf("writing field: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}
	if got := postImport(t, w.FormDataContentType(), &body); !strings.Contains(got, "dump file is required") {
		t.Fatalf("body = %q", got)
	}
}

func TestDatabaseImportRejectsNonMultipartBody(t *testing.T) {
	if got := postImport(t, "application/json", bytes.NewBufferString("{}")); !strings.Contains(got, "dump file is required") {
		t.Fatalf("body = %q", got)
	}
}
