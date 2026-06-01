package ui

import (
	"encoding/json"
	"net/http"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/devtoolsops"
)

// handleDevtoolsStatus reports whether the lerd_devtools collector is armed.
// Capture events themselves flow over the shared dumps receiver, so the
// listening/count/buffer numbers live on /api/dumps/status; this endpoint is
// only the enable toggle's state.
func handleDevtoolsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buildDevtoolsStatusJSON())
}

func buildDevtoolsStatusJSON() []byte {
	cfg, _ := config.LoadGlobal()
	// Enable state is shared with the debug bridge (one flag), so it mirrors
	// Dumps.Enabled; only the worker-capture sub-toggle is devtools-specific.
	resp := struct {
		Enabled bool `json:"enabled"`
		Workers bool `json:"workers"`
	}{
		Enabled: cfg != nil && cfg.IsDumpsEnabled(),
		Workers: cfg != nil && cfg.IsDevtoolsWorkers(),
	}
	b, _ := json.Marshal(resp)
	return b
}

// handleDevtoolsWorkers toggles capture of queue/scheduler worker queries.
// Loopback-only, same trust boundary as the enable toggle.
func handleDevtoolsWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	res, err := devtoolsops.SetWorkers(req.Enable)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, res)
}
