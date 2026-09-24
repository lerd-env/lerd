package ui

import (
	"encoding/json"
	"net/http"

	"github.com/geodro/lerd/internal/config"
)

const errStreamingDisabled = "streaming mode is disabled, enable it in Lerd settings or with lerd streaming enable"

// streamingState reports whether streaming mode is on and which sites it hides.
func streamingState(cfg *config.GlobalConfig) (bool, map[string]bool) {
	if !cfg.Streaming() {
		return false, map[string]bool{}
	}
	reg, err := config.LoadSites()
	if err != nil {
		return true, map[string]bool{}
	}
	return true, cfg.StreamingHidden(reg)
}

// privateWorkspaceNames lists the workspaces flagged private, so the dashboard
// can label the toggle on each one. While streaming they are hidden anyway.
func privateWorkspaceNames(cfg *config.GlobalConfig) []string {
	names := []string{}
	if cfg == nil || cfg.Streaming() {
		return names
	}
	for _, w := range cfg.Workspaces {
		if w.Private {
			names = append(names, w.Name)
		}
	}
	return names
}

// handleSettingsStreaming turns streaming mode on or off. The sites and status
// snapshots rebuild after it, so every open dashboard drops or regains the
// private sites and workspaces at once.
func handleSettingsStreaming(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if body.Enabled {
		if cfg, _ := config.LoadGlobal(); cfg == nil || !cfg.UI.StreamingEnabled {
			writeJSON(w, map[string]any{"ok": false, "error": errStreamingDisabled})
			return
		}
		broker.broadcastStreamingOn()
	}
	if err := config.SetStreamingMode(body.Enabled); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "enabled": body.Enabled})
}

// handleSettingsStreamingEnabled opts in or out of streaming mode as a whole.
// Disabling it clears the mode too, so nothing stays hidden.
func handleSettingsStreamingEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := config.SetStreamingEnabled(body.Enabled); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "enabled": body.Enabled})
}

// streamingHiddenNow returns the sites streaming mode hides right now and their
// domains, both empty while it is off.
func streamingHiddenNow() (map[string]bool, map[string]bool) {
	cfg, _ := config.LoadGlobal()
	reg, err := config.LoadSites()
	if err != nil {
		return map[string]bool{}, map[string]bool{}
	}
	hidden := cfg.StreamingHidden(reg)
	return hidden, config.HiddenDomains(reg, hidden)
}

// hideStreamingServices drops the workers a hidden site owns and its domains
// from the services a service is wired to, so no card names a private site.
func hideStreamingServices(list []ServiceResponse, hidden, domains map[string]bool) []ServiceResponse {
	if len(hidden) == 0 {
		return list
	}
	out := make([]ServiceResponse, 0, len(list))
	for _, s := range list {
		owner := s.WorkerSite + s.QueueSite + s.ScheduleWorkerSite + s.ReverbSite + s.HorizonSite + s.StripeListenerSite
		if hidden[owner] {
			continue
		}
		if len(s.SiteDomains) > 0 {
			kept := []string{}
			for _, d := range s.SiteDomains {
				if !config.DomainHidden(d, domains) {
					kept = append(kept, d)
				}
			}
			s.SiteDomains = kept
		}
		out = append(out, s)
	}
	return out
}

// hideStreamingDatabases drops the databases a hidden site or its worktrees
// own, and the snapshots listed under them with them.
func hideStreamingDatabases(engines []dbEngineResponse, hidden, domains map[string]bool) []dbEngineResponse {
	if len(hidden) == 0 {
		return engines
	}
	for i := range engines {
		kept := []dbEntryResponse{}
		for _, db := range engines[i].Databases {
			if !config.EntityHidden(db.Name, db.Site, hidden, domains) {
				kept = append(kept, db)
			}
		}
		engines[i].Databases = kept
	}
	return engines
}

// hideStreamingEntityRows drops the buckets, keyspaces and other entities a
// hidden site owns.
func hideStreamingEntityRows(kinds []entityKindResponse, hidden, domains map[string]bool) []entityKindResponse {
	if len(hidden) == 0 {
		return kinds
	}
	for i := range kinds {
		kept := []entityRowResponse{}
		for _, row := range kinds[i].Rows {
			if !config.EntityHidden(row.Name, row.Site, hidden, domains) {
				kept = append(kept, row)
			}
		}
		kinds[i].Rows = kept
	}
	return kinds
}

// hideStreamingAutoSnapshot drops a hidden site from the snapshot schedule list.
func hideStreamingAutoSnapshot(resp autoSnapshotResponse, hidden map[string]bool) autoSnapshotResponse {
	if len(hidden) == 0 {
		return resp
	}
	kept := []autoSnapshotSiteStatus{}
	for _, s := range resp.Sites {
		if !hidden[s.Site] {
			kept = append(kept, s)
		}
	}
	resp.Sites = kept
	return resp
}
