package ui

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceRecentLogs_unknownUnit(t *testing.T) {
	result := serviceRecentLogs("lerd-nonexistent-unit-xyz")
	if len(result) > 200 {
		t.Errorf("expected short/empty result for unknown unit, got %d bytes", len(result))
	}
}

func TestIsContainerUnit_nginx(t *testing.T) {
	// lerd-nginx in its container, the default; on macOS the plist says which,
	// so the test brings its own rather than reading the developer's.
	home := t.TempDir()
	t.Setenv("HOME", home)
	agents := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "lerd-nginx.plist"), []byte("<plist><dict><key>Label</key><string>lerd-nginx</string></dict></plist>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isContainerUnit("lerd-nginx") {
		t.Error("expected isContainerUnit to return true for lerd-nginx")
	}
}

func TestIsContainerUnit_dns(t *testing.T) {
	// lerd-dns runs as a host service everywhere, so its logs come from the
	// journal or launchd log file, never podman.
	if isContainerUnit("lerd-dns") {
		t.Error("expected isContainerUnit to return false for lerd-dns")
	}
}

// streamUnitLogs must send the response headers and an initial SSE comment
// before running the log-following subprocess; otherwise silent units leave
// the browser's EventSource stuck in CONNECTING and the UI sticks on
// "connecting...".
func TestStreamUnitLogs_flushesHeadersBeforeStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/schedule/nonexistent/logs", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		streamUnitLogs(rec, req, "lerd-nonexistent-unit-xyz")
		close(done)
	}()
	<-done

	if ct := rec.Result().Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, ": connected") {
		n := len(body)
		if n > 40 {
			n = 40
		}
		t.Errorf("body should start with initial SSE comment, got: %q", body[:n])
	}
}
