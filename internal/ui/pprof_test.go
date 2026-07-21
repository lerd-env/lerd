package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// isolatePprofState points the marker at a temp run dir. Without it these
// tests create and delete the marker in the developer's live install, and an
// interrupted run leaves profiling switched on for a real daemon.
func isolatePprofState(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

// enablePprof creates the marker for the duration of a test.
func enablePprof(t *testing.T) {
	t.Helper()
	isolatePprofState(t)
	if err := os.MkdirAll(config.RunDir(), 0755); err != nil {
		t.Fatal(err)
	}
	path := config.PprofMarkerPath()
	config.GuardRealWrite(path)
	if err := os.WriteFile(path, []byte("1\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func pprofRequest(target, remoteAddr string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.RemoteAddr = remoteAddr
	return r
}

// Profiling stays invisible until it is explicitly unlocked, and a 404 rather
// than a 403 keeps a disabled daemon from advertising that the surface exists.
func TestPprof_DisabledByDefault(t *testing.T) {
	isolatePprofState(t)

	w := httptest.NewRecorder()
	handlePprof(w, pprofRequest("/debug/pprof/", "127.0.0.1:5000"))

	if w.Code != http.StatusNotFound {
		t.Errorf("without the marker the endpoint must 404, got %d", w.Code)
	}
}

// lerd-ui binds 0.0.0.0 so LAN clients can reach it when lan:expose is on.
// Profiling dumps goroutine stacks, the command line, and heap contents, so
// the marker alone must never be enough: off-host callers stay locked out.
func TestPprof_RejectsNonLoopbackEvenWhenEnabled(t *testing.T) {
	enablePprof(t)

	w := httptest.NewRecorder()
	handlePprof(w, pprofRequest("/debug/pprof/", "192.168.1.50:5000"))

	if w.Code != http.StatusNotFound {
		t.Errorf("a LAN client must not reach pprof, got %d", w.Code)
	}
}

func TestPprof_ServesLoopbackWhenEnabled(t *testing.T) {
	enablePprof(t)

	w := httptest.NewRecorder()
	handlePprof(w, pprofRequest("/debug/pprof/", "127.0.0.1:5000"))

	if w.Code != http.StatusOK {
		t.Fatalf("loopback request with the marker present should serve, got %d", w.Code)
	}
	if body := w.Body.String(); body == "" {
		t.Error("expected the pprof index to render")
	}
}

// The named runtime profiles route through the same gate.
func TestPprof_ServesNamedProfile(t *testing.T) {
	enablePprof(t)

	w := httptest.NewRecorder()
	handlePprof(w, pprofRequest("/debug/pprof/goroutine?debug=1", "127.0.0.1:5000"))

	if w.Code != http.StatusOK {
		t.Fatalf("goroutine profile should serve, got %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("expected goroutine profile output")
	}
}
