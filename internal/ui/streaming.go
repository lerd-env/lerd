package ui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/stats"
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
	domains := map[string]bool{}
	for _, s := range reg.Sites {
		if hidden[s.Name] {
			for _, d := range s.Domains {
				domains[d] = true
			}
		}
	}
	return hidden, domains
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
				if !domains[d] {
					kept = append(kept, d)
				}
			}
			s.SiteDomains = kept
		}
		out = append(out, s)
	}
	return out
}

// hideStreamingContainers drops a hidden site's containers from the resource
// stats. They are named lerd-<worker>-<site>, with -<worktree> for a worktree.
func hideStreamingContainers(snap stats.Snapshot, hidden map[string]bool) stats.Snapshot {
	if len(hidden) == 0 {
		return snap
	}
	kept := make([]stats.ContainerStat, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		if !containerOfHiddenSite(c.Name, hidden) {
			kept = append(kept, c)
		}
	}
	snap.Containers = kept
	return snap
}

func containerOfHiddenSite(name string, hidden map[string]bool) bool {
	for site := range hidden {
		if strings.HasSuffix(name, "-"+site) || strings.Contains(name, "-"+site+"-") {
			return true
		}
	}
	return false
}
