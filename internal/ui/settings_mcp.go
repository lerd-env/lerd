package ui

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/geodro/lerd/internal/cli"
)

// Registration runs through these so tests never write into the user-scope
// config of the AI assistants on the machine running them.
var (
	mcpEnable     = cli.RunMCPEnableGlobal
	mcpDisable    = cli.RunMCPDisableGlobal
	mcpConfigured = func() bool {
		home, err := os.UserHomeDir()
		return err == nil && cli.MCPGlobalConfigured(home)
	}
)

// handleSettingsMCP registers lerd's MCP server with every supported AI
// assistant, or removes it, exactly as mcp:enable-global and mcp:disable-global do.
func handleSettingsMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	apply := mcpDisable
	if body.Enabled {
		apply = mcpEnable
	}
	if err := apply(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "mcp_global": body.Enabled})
}
