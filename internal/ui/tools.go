package ui

import (
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/tools"
)

// Seams so the handlers can be tested without a download or the network.
var (
	updateToolFn  = cli.UpdateOneTool
	refreshPinsFn = tools.Refresh
	toolStatusFn  = tools.StatusAll
)

// handleTools serves the managed host tools under /api/tools/:
//
//	POST /api/tools/check           re-read the pins, bypassing the 24h cache
//	POST /api/tools/<name>/update   apply one tool's pinned version
//
// The route is registered as a prefix, so the action is matched rather than
// assumed: anything else under it is a 404, not a silent install.
func handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/tools/")
	if rest == "check" {
		handleToolCheck(w, r)
		return
	}
	name, ok := strings.CutSuffix(rest, "/update")
	if !ok || name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	handleToolUpdate(w, name)
}

// handleToolCheck re-reads the published pins with the cache bypassed and
// answers with the tools as they stand afterwards. The pins live in one
// manifest, so this is a single check for all of them rather than one per card.
func handleToolCheck(w http.ResponseWriter, r *http.Request) {
	refreshPinsFn(r.Context())
	writeJSON(w, map[string]any{"ok": true, "tools": toolStatusFn(r.Context())})
}

// handleToolUpdate applies the pinned version of one host tool. One tool per
// call rather than a batch, because each tool card renders its own outcome: a
// download whose checksum does not match is refused deliberately, and the card
// has to name that tool rather than show a failure for all of them.
func handleToolUpdate(w http.ResponseWriter, name string) {
	if err := updateToolFn(name); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
