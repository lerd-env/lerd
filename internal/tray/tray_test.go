//go:build !nogui

package tray

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestOffOn_FlipsState(t *testing.T) {
	if got := offOn(true); got != "off" {
		t.Errorf("offOn(true) = %q, want %q", got, "off")
	}
	if got := offOn(false); got != "on" {
		t.Errorf("offOn(false) = %q, want %q", got, "on")
	}
}

func TestRunAndRefresh_CallsRefreshAfterCommand(t *testing.T) {
	called := false
	runAndRefresh(exec.Command("true"), func() { called = true })
	if !called {
		t.Error("refresh should be called after command exits")
	}
}

func TestRunAndRefresh_CallsRefreshEvenWhenCommandFails(t *testing.T) {
	called := false
	runAndRefresh(exec.Command("false"), func() { called = true })
	if !called {
		t.Error("refresh should be called even if the command fails so the menu still redraws")
	}
}

func TestLerdBin_FallbackWarnsOnceAndReturnsBareName(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")

	prevLook := lerdBinLookPath
	prevCands := lerdBinCandidates
	prevW := lerdBinWarnW
	lerdBinLookPath = func(string) (string, error) { return "", errors.New("not on PATH") }
	lerdBinCandidates = func() []string { return []string{missing} }
	var buf bytes.Buffer
	lerdBinWarnW = &buf
	lerdBinFallbackWarn = sync.Once{}
	t.Cleanup(func() {
		lerdBinLookPath = prevLook
		lerdBinCandidates = prevCands
		lerdBinWarnW = prevW
		lerdBinFallbackWarn = sync.Once{}
	})

	if got := lerdBin(); got != "lerd" {
		t.Errorf("first call: lerdBin() = %q, want %q (bare fallback)", got, "lerd")
	}
	if !strings.Contains(buf.String(), "could not locate the lerd binary") {
		t.Errorf("expected fallback warning on stderr, got: %q", buf.String())
	}
	first := buf.Len()

	if got := lerdBin(); got != "lerd" {
		t.Errorf("second call: lerdBin() = %q, want %q", got, "lerd")
	}
	if buf.Len() != first {
		t.Errorf("sync.Once should suppress repeat warnings; buffer grew from %d to %d", first, buf.Len())
	}
}

func TestLerdBin_PrefersLookPathHit(t *testing.T) {
	prevLook := lerdBinLookPath
	prevCands := lerdBinCandidates
	lerdBinLookPath = func(string) (string, error) { return "/opt/from-path/lerd", nil }
	lerdBinCandidates = func() []string {
		t.Error("candidates must not be consulted when LookPath succeeds")
		return nil
	}
	t.Cleanup(func() {
		lerdBinLookPath = prevLook
		lerdBinCandidates = prevCands
	})

	if got := lerdBin(); got != "/opt/from-path/lerd" {
		t.Errorf("lerdBin() = %q, want LookPath result", got)
	}
}

// macOS has none of the Linux terminals the tray tries, so the update item did
// nothing there. It now asks lerd-ui, which opens Terminal or iTerm. Issue #2414.
func TestOpenUpdateTerminal_FallsBackToLerdUI(t *testing.T) {
	var gotMethod, gotCSRF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/lerd/update-terminal" {
			http.NotFound(w, r)
			return
		}
		gotMethod, gotCSRF = r.Method, r.Header.Get("X-Lerd-CSRF")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	prevBase, prevLook := apiBase, lookPathFn
	t.Cleanup(func() { apiBase, lookPathFn = prevBase, prevLook })
	apiBase = srv.URL
	lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }

	openUpdateTerminal("9.9.9")
	if gotMethod != http.MethodPost || gotCSRF == "" {
		t.Errorf("lerd-ui got method %q csrf %q, want a POST carrying the CSRF header", gotMethod, gotCSRF)
	}
}

// A failure is written to the tray's log instead of vanishing.
func TestOpenUpdateTerminal_LogsWhenNothingOpens(t *testing.T) {
	prevLook, prevUI, prevW := lookPathFn, uiUpdateTerminal, lerdBinWarnW
	t.Cleanup(func() { lookPathFn, uiUpdateTerminal, lerdBinWarnW = prevLook, prevUI, prevW })
	lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
	uiUpdateTerminal = func() error { return errors.New("no terminal emulator found") }
	var log bytes.Buffer
	lerdBinWarnW = &log

	openUpdateTerminal("9.9.9")
	if !strings.Contains(log.String(), "no terminal emulator found") {
		t.Errorf("log = %q, want the reason nothing opened", log.String())
	}
}

// The tray asks lerd-ui to start or stop rather than running `lerd start` in
// its own process group: the start relaunches the tray, and as the tray's child
// it was killed along with it, leaving no tray at all.
func TestLifecycleViaUI_PostsToTheDaemon(t *testing.T) {
	var gotPath, gotCSRF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotPath, gotCSRF = r.URL.Path, r.Header.Get("X-Lerd-CSRF")
		}
		w.Write([]byte(`{"phase":"done"}` + "\n"))
	}))
	defer srv.Close()
	prev := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = prev })

	if reached, err := lifecycleViaUI("start"); !reached || err != nil {
		t.Fatalf("reached %v err %v, want a reached daemon and no error", reached, err)
	}
	if gotPath != "/api/lerd/start" || gotCSRF != "1" {
		t.Errorf("posted %q with csrf %q", gotPath, gotCSRF)
	}
}

func TestLifecycleViaUI_ReportsAnUnreachableDaemon(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	prev := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = prev })

	if reached, _ := lifecycleViaUI("stop"); reached {
		t.Error("an unreachable daemon must leave the command to the CLI")
	}
}

// A start that streams a failed event, or a stop that answers ok:false, is a
// failure: the tray must not go on to show the state it asked for.
func TestLifecycleViaUI_CarriesTheDaemonsFailureBack(t *testing.T) {
	for _, body := range []string{
		`{"phase":"step","step":"units"}` + "\n" + `{"phase":"failed","error":"nginx would not start"}` + "\n",
		`{"ok":false,"error":"podman is not running"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(body))
		}))
		prev := apiBase
		apiBase = srv.URL
		reached, err := lifecycleViaUI("start")
		apiBase = prev
		srv.Close()
		if !reached || err == nil {
			t.Errorf("body %q: reached %v err %v, want the failure reported", body, reached, err)
		}
	}
}
