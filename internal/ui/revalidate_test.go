package ui

import (
	"net/http/httptest"
	"testing"
)

// The icons and marks barely change between loads, so a browser asks whether
// its copy still holds and gets an empty 304 when it does.
func TestWriteJSONRevalidated_AnswersNotModifiedForTheSameBody(t *testing.T) {
	body := map[string]string{"mysql": "<svg/>"}
	first := httptest.NewRecorder()
	writeJSONRevalidated(first, httptest.NewRequest("GET", "/api/services/icons", nil), body)
	etag := first.Header().Get("ETag")
	if first.Code != 200 || etag == "" || first.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("first = %d etag %q cache %q", first.Code, etag, first.Header().Get("Cache-Control"))
	}

	again := httptest.NewRequest("GET", "/api/services/icons", nil)
	again.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	writeJSONRevalidated(rec, again, body)
	if rec.Code != 304 || rec.Body.Len() != 0 {
		t.Errorf("revalidated = %d with %d bytes, want an empty 304", rec.Code, rec.Body.Len())
	}

	changed := httptest.NewRequest("GET", "/api/services/icons", nil)
	changed.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	writeJSONRevalidated(rec, changed, map[string]string{"mysql": "<svg/>", "redis": "<svg/>"})
	if rec.Code != 200 || rec.Header().Get("ETag") == etag {
		t.Errorf("changed body = %d etag %q, want a fresh 200", rec.Code, rec.Header().Get("ETag"))
	}
}
