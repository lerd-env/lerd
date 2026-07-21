package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnitForLogPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/api/logs/lerd-nginx", "lerd-nginx"},
		{"/api/logs/lerd-php84-fpm", "lerd-php84-fpm"},
		{"/api/watcher/logs", "lerd-watcher"},
		{"/api/queue/alpha/logs", "lerd-queue-alpha"},
		{"/api/horizon/alpha/logs", "lerd-horizon-alpha"},
		{"/api/schedule/alpha/logs", "lerd-schedule-alpha"},
		{"/api/reverb/alpha/logs", "lerd-reverb-alpha"},
		{"/api/stripe/alpha/logs", "lerd-stripe-alpha"},
		{"/api/worker/alpha/vite/logs", "lerd-vite-alpha"},
		{"/api/worker/alpha-feature/app/logs", "lerd-app-alpha-feature"},
	}
	for _, c := range cases {
		got, ok := unitForLogPath(c.path)
		if !ok || got != c.want {
			t.Errorf("unitForLogPath(%q) = %q, %v; want %q", c.path, got, ok, c.want)
		}
	}
}

func TestUnitForLogPath_Rejects(t *testing.T) {
	for _, p := range []string{
		"",
		"/api/logs/",
		"/api/logs/nginx",               // not a lerd- unit
		"/api/logs/lerd-nginx;rm -rf /", // shell metacharacters
		"/api/logs/lerd-nginx/../etc",
		"/api/queue//logs",
		"/api/worker/alpha/logs",
		"/api/app-logs/alpha",
		"http://evil/api/logs/lerd-nginx",
	} {
		if got, ok := unitForLogPath(p); ok {
			t.Errorf("unitForLogPath(%q) = %q, want rejected", p, got)
		}
	}
}

func TestHandleLogTerminal_MethodAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	handleLogTerminal(rec, httptest.NewRequest(http.MethodGet, "/api/logs/terminal", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	body := strings.NewReader(`{"path":"/api/logs/not-a-unit"}`)
	handleLogTerminal(rec, httptest.NewRequest(http.MethodPost, "/api/logs/terminal", body))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown unit = %d, want 404", rec.Code)
	}
}

func TestHandleLogTerminal_ReportsMissingEmulator(t *testing.T) {
	// With an empty PATH no emulator resolves, so the handler must report the
	// failure as JSON rather than pretending it opened one.
	t.Setenv("PATH", "")
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"path":"/api/logs/lerd-nginx"}`)
	handleLogTerminal(rec, httptest.NewRequest(http.MethodPost, "/api/logs/terminal", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Errorf("body = %s, want ok:false", rec.Body.String())
	}
}

func TestLogFollowScript_MentionsUnit(t *testing.T) {
	script := logFollowScript("lerd-nginx")
	if !strings.Contains(script, "lerd-nginx") {
		t.Errorf("logFollowScript = %q, want it to reference the unit", script)
	}
}
