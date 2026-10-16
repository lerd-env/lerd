package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/geodro/lerd/internal/desktopnotify"
	"github.com/geodro/lerd/internal/tray"
)

// stubTray serves a fixed menu, records the commands an action runs instead of
// running them, and captures the desktop notifications they raise.
type stubTray struct {
	mu      sync.Mutex
	ran     [][]string
	updates int
	toasts  []desktopnotify.Request
	err     error
}

func useStubTray(t *testing.T, snap *tray.Snapshot) *stubTray {
	t.Helper()
	s := &stubTray{}
	prevMenu, prevRun, prevUpdate := readTrayMenu, runTrayCommand, openTrayUpdate
	prevEmit, prevSupported := emitDesktopNotification, desktopSupported
	readTrayMenu = func() tray.Menu { return tray.BuildMenu(snap, tray.MenuOptions{LANToggle: true}) }
	runTrayCommand = func(args []string) (string, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ran = append(s.ran, args)
		if s.err != nil {
			return "first line\n" + s.err.Error() + "\n", s.err
		}
		return "", nil
	}
	openTrayUpdate = func() error { s.updates++; return nil }
	emitDesktopNotification = func(r desktopnotify.Request) (uint32, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.toasts = append(s.toasts, r)
		return 0, nil
	}
	desktopSupported = func() bool { return true }
	t.Cleanup(func() {
		readTrayMenu, runTrayCommand, openTrayUpdate = prevMenu, prevRun, prevUpdate
		emitDesktopNotification, desktopSupported = prevEmit, prevSupported
	})
	return s
}

func trayTestSnapshot() *tray.Snapshot {
	return &tray.Snapshot{
		Running:       true,
		NginxRunning:  true,
		ServicesKnown: true,
		Services:      nil,
		PHPDefault:    "8.4",
	}
}

func postTrayAction(id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/tray/action", strings.NewReader(`{"action":"`+id+`"}`))
	rec := httptest.NewRecorder()
	handleTrayAction(rec, req)
	return rec
}

func TestHandleTray_ServesTheResolvedMenu(t *testing.T) {
	useStubTray(t, trayTestSnapshot())
	rec := httptest.NewRecorder()
	handleTray(rec, httptest.NewRequest(http.MethodGet, "/api/tray", nil))

	var m struct {
		Running bool `json:"running"`
		Items   []struct {
			Label  string `json:"label"`
			Action string `json:"action"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	if !m.Running || len(m.Items) == 0 {
		t.Errorf("menu = %+v", m)
	}
}

func TestHandleTrayAction_RunsOnlyWhatTheMenuOffers(t *testing.T) {
	s := useStubTray(t, trayTestSnapshot())

	if rec := postTrayAction("notify on"); rec.Code != http.StatusOK {
		t.Fatalf("notify on: status %d %s", rec.Code, rec.Body)
	}
	for _, id := range []string{"notify off", "uninstall", "service start x; rm -rf ~", ""} {
		if rec := postTrayAction(id); rec.Code != http.StatusConflict {
			t.Errorf("%q: status %d, want %d", id, rec.Code, http.StatusConflict)
		}
	}
	if len(s.ran) != 1 || strings.Join(s.ran[0], " ") != "notify on" {
		t.Errorf("ran %v, want only [notify on]", s.ran)
	}
}

func TestHandleTrayAction_RejectsGet(t *testing.T) {
	useStubTray(t, trayTestSnapshot())
	rec := httptest.NewRecorder()
	handleTrayAction(rec, httptest.NewRequest(http.MethodGet, "/api/tray/action", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d", rec.Code)
	}
}

type trayActionResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func decodeTrayAction(t *testing.T, rec *httptest.ResponseRecorder) trayActionResponse {
	t.Helper()
	var resp trayActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	return resp
}

// The client that drew the menu reports the outcome its own way, so lerd-ui
// hands it the message instead of raising a notification itself.
func TestHandleTrayAction_ReturnsTheOutcomeForTheClient(t *testing.T) {
	s := useStubTray(t, trayTestSnapshot())

	ok := decodeTrayAction(t, postTrayAction("dump on"))
	s.err = errors.New("dumps need lerd running")
	bad := decodeTrayAction(t, postTrayAction("dump on"))

	if want := (trayActionResponse{OK: true, Title: "Dumps on"}); ok != want {
		t.Errorf("success = %+v, want %+v", ok, want)
	}
	if want := (trayActionResponse{Error: "dumps need lerd running", Title: "Couldn't turn dumps on", Body: "dumps need lerd running"}); bad != want {
		t.Errorf("failure = %+v, want %+v with the last line lerd printed", bad, want)
	}
	if len(s.toasts) != 0 {
		t.Errorf("lerd-ui raised %+v; the client shows the outcome", s.toasts)
	}
}

func TestHandleTrayAction_UpdateOpensATerminal(t *testing.T) {
	snap := trayTestSnapshot()
	snap.LatestVersion = "v9.9.9"
	s := useStubTray(t, snap)

	postTrayAction(tray.ActionUpdate)
	if s.updates != 1 || len(s.ran) != 0 {
		t.Errorf("updates = %d, ran %v; want the update terminal and no lerd command", s.updates, s.ran)
	}
}

func TestHandleTrayAction_UpToDateOnlySaysSo(t *testing.T) {
	s := useStubTray(t, trayTestSnapshot())

	resp := decodeTrayAction(t, postTrayAction(tray.ActionUpdate))
	if s.updates != 0 || !resp.OK || resp.Title != "Lerd is up to date" {
		t.Errorf("updates = %d, response = %+v", s.updates, resp)
	}
}

func TestTrayActionIsLoopbackOnly(t *testing.T) {
	if !isLoopbackOnlyPath("/api/tray/action") {
		t.Error("/api/tray/action stops lerd and runs its commands, so it must stay with the local user")
	}
}
