package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/traymenu"
	lerdUpdate "github.com/geodro/lerd/internal/update"
	"github.com/geodro/lerd/internal/version"
)

// trayGet answers the tray's API reads in process, from the handlers a tray
// polling over HTTP would reach, so both see the same state.
func trayGet(path string) (io.ReadCloser, error) {
	var h http.HandlerFunc
	switch path {
	case "/api/status":
		h = handleStatus
	case "/api/services":
		h = handleServices
	case "/api/workers/health":
		h = handleWorkersHealth
	default:
		return nil, os.ErrNotExist
	}
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return io.NopCloser(rec.Body), nil
}

// Seams for tests: the menu, the lerd commands an action runs, and the update
// terminal.
var (
	readTrayMenu = func() traymenu.Menu {
		return traymenu.ReadMenu(trayGet, traymenu.LocalSettings, traymenu.MenuOptions{LANToggle: !platform.Current.UsesMachineVM})
	}
	runTrayCommand = runLerdCommand
	openTrayUpdate = func() error {
		self, err := os.Executable()
		if err != nil || self == "" {
			self = "lerd"
		}
		return openTerminalCommand(buildUpdateScript(self))
	}
	// The menu's answer is up to a day old, and the user asked for a live one.
	trayUpdateAvailable = func() bool {
		info, _ := lerdUpdate.ForceUpdateCheck(version.Version)
		return info != nil
	}
)

// handleTray serves the tray menu resolved from lerd's state, for trays drawn
// outside lerd: the desktop app, or a status bar module.
func handleTray(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, readTrayMenu())
}

// trayActionResult is the outcome of a tray action, worded for the client to
// show as its own notification. Title is empty when there is nothing to say.
type trayActionResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// handleTrayAction runs one action of the current menu and answers with what
// to tell the user. An id the menu does not offer, including a toggle that has
// flipped since the client drew it, is a conflict.
func handleTrayAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	menu := readTrayMenu()
	item, ok := menu.Find(body.Action)
	if !ok {
		http.Error(w, "the tray menu has no action "+body.Action, http.StatusConflict)
		return
	}

	if item.Action == traymenu.ActionUpdate {
		if menu.LatestVersion == "" && !trayUpdateAvailable() {
			writeJSON(w, trayActionResult{OK: true, Title: "Lerd is up to date"})
			return
		}
		if err := openTrayUpdate(); err != nil {
			writeJSON(w, trayActionResult{Error: err.Error(), Title: "Couldn't open the update", Body: err.Error()})
			return
		}
		writeJSON(w, trayActionResult{OK: true})
		return
	}

	// Quitting takes this process down with it, so the answer goes first.
	if item.Args[0] == "quit" {
		writeJSON(w, trayActionResult{OK: true, Title: "Lerd stopped"})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go runTrayCommand(item.Args) //nolint:errcheck
		return
	}

	stderr, err := runTrayCommand(item.Args)
	res := trayActionResult{OK: err == nil}
	if title, msg, show := traymenu.ActionMessage(item.Args, err, stderr); show {
		res.Title, res.Body = title, msg
	}
	if err != nil {
		res.Error = res.Body
	}
	writeJSON(w, res)
}

// runLerdCommand runs one tray action. Start, stop and quit go through the
// same calls as the dashboard's buttons; the rest run this lerd binary.
func runLerdCommand(args []string) (string, error) {
	switch strings.Join(args, " ") {
	case "start":
		return "", startLerd(func(cli.StartEvent) {})
	case "stop":
		return "", stopLerd()
	case "quit":
		return "", cli.RunQuit()
	}
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "lerd"
	}
	var stderr bytes.Buffer
	cmd := exec.Command(self, args...)
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stderr.String(), err
}
