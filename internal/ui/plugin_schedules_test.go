package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/plugins"
)

func declareSchedule(t *testing.T) {
	t.Helper()
	path := filepath.Join(plugins.Dir(), "echo", plugins.ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, "schedules:\n  - {name: sweep, every: 1h, run: bin/say}\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Running on a schedule is the user's to allow, from the dashboard, never by
// the plugin's own token.
func TestPluginManager_ScheduleAnswerIsTheUsers(t *testing.T) {
	notifyGateConfigDir(t)
	installAPIPlugin(t)
	declareSchedule(t)
	answer := func(withToken bool, body string) int {
		req := localRequest(http.MethodPost, "/api/plugin-manager/echo/schedule", body)
		if withToken {
			token, _ := plugins.Token("echo")
			req.Header.Set(plugins.TokenHeader, token)
		}
		rec := httptest.NewRecorder()
		handlePluginManager(rec, req)
		return rec.Code
	}
	state := func() string {
		p, _ := plugins.Find("echo")
		return plugins.PermissionState(p, plugins.PermissionSchedule)
	}
	if code := answer(true, `{"allowed":true}`); code != http.StatusForbidden {
		t.Fatalf("with the plugin's token: %d, want 403", code)
	}
	if got := state(); got != plugins.PermissionPending {
		t.Fatalf("state after a refused answer = %q, want pending", got)
	}
	if code := answer(false, `not json`); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d, want 400", code)
	}
	if code := answer(false, `{"allowed":true}`); code != http.StatusOK {
		t.Fatalf("from the dashboard: %d, want 200", code)
	}
	if got := state(); got != plugins.PermissionAllowed {
		t.Fatalf("state = %q, want allowed", got)
	}
}

// The plugin manager lists where the plugin stands and each schedule, for its
// card to show and switch.
func TestManagedPlugins_CarryScheduleStateAndList(t *testing.T) {
	notifyGateConfigDir(t)
	installAPIPlugin(t)
	declareSchedule(t)
	var row managedPlugin
	for _, p := range managedPlugins() {
		if p.Name == "echo" {
			row = p
		}
	}
	if row.Schedule != plugins.PermissionPending {
		t.Errorf("schedule = %q, want pending", row.Schedule)
	}
	if len(row.Schedules) != 1 || row.Schedules[0].Name != "sweep" || row.Schedules[0].Every != "1h" {
		t.Errorf("schedules = %+v", row.Schedules)
	}
}
