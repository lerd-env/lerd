package watcher

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Profiling stays off until the marker exists, checked per request so a
// misbehaving watcher can be profiled without the restart that would reset it.
func TestPprofHandler_followsTheMarker(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	get := func() int {
		rec := httptest.NewRecorder()
		pprofHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil))
		return rec.Code
	}
	if code := get(); code != http.StatusNotFound {
		t.Fatalf("without the marker got %d, want 404", code)
	}
	if err := os.MkdirAll(filepath.Dir(config.PprofMarkerPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.PprofMarkerPath(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := get(); code != http.StatusOK {
		t.Fatalf("with the marker got %d, want 200", code)
	}
}
