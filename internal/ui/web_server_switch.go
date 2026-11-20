package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/geodro/lerd/internal/cli"
)

type webServerSwitchResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Output string `json:"output,omitempty"`
}

// webServerSwitchFn is the CLI's own switch, so the dashboard checks, pulls,
// restarts and rolls back exactly as lerd web-server does; swapped in tests.
var (
	webServerSwitchFn = cli.SwitchWebServer
	webServerSwitchMu sync.Mutex
	// afterWebServerSwitch drops and republishes the cached status, so every
	// client sees which server serves now; swapped in tests.
	afterWebServerSwitch = publishStatus
)

// handleWebServerSwitch serves POST /api/web-server {"kind": "nginx"|"go"}.
// One switch runs at a time, and the switch's own words come back as output.
func handleWebServerSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		writeJSON(w, webServerSwitchResponse{Error: "invalid body: " + err.Error()})
		return
	}
	webServerSwitchMu.Lock()
	defer webServerSwitchMu.Unlock()
	defer afterWebServerSwitch()
	var out bytes.Buffer
	if err := webServerSwitchFn(req.Kind, io.Writer(&out)); err != nil {
		writeJSON(w, webServerSwitchResponse{Error: err.Error(), Output: out.String()})
		return
	}
	writeJSON(w, webServerSwitchResponse{OK: true, Output: out.String()})
}
