package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// autoSnapshotResponse is the whole automatic-snapshot surface in one payload:
// the global policy and every site a database could be snapshotted for, covered
// or not, so the opt-in and the opt-out are offered in the same place.
type autoSnapshotResponse struct {
	Enabled bool   `json:"enabled"`
	Every   string `json:"every"`
	Keep    int    `json:"keep"`
	KeepFor string `json:"keep_for"`
	// Selection is "opt_out" (cover every site until one is excluded) or
	// "opt_in" (cover nothing until a site is included).
	Selection string                   `json:"selection"`
	Sites     []autoSnapshotSiteStatus `json:"sites"`
}

type autoSnapshotSiteStatus struct {
	Site     string     `json:"site"`
	Domain   string     `json:"domain,omitempty"`
	Service  string     `json:"service"`
	Database string     `json:"database"`
	Mode     string     `json:"mode"`
	Covered  bool       `json:"covered"`
	Last     *time.Time `json:"last,omitempty"`
	Next     *time.Time `json:"next,omitempty"`
}

// handleAutoSnapshot reads the policy (GET) or saves it (POST).
func handleAutoSnapshot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, autoSnapshotStatus())
	case http.MethodPost:
		var body struct {
			Enabled   bool   `json:"enabled"`
			Every     string `json:"every"`
			Keep      int    `json:"keep"`
			KeepFor   string `json:"keep_for"`
			Selection string `json:"selection"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeDBError(w, "invalid request")
			return
		}
		if body.Every != "" {
			if _, err := time.ParseDuration(body.Every); err != nil {
				writeDBError(w, "the schedule must be a duration like 6h or 24h")
				return
			}
		}
		if body.KeepFor != "" {
			if _, err := time.ParseDuration(body.KeepFor); err != nil {
				writeDBError(w, "the maximum age must be a duration like 168h")
				return
			}
		}
		selection, err := config.NormalizeAutoSnapshotSelection(body.Selection)
		if err != nil {
			writeDBError(w, err.Error())
			return
		}
		cfg, err := config.LoadGlobal()
		if err != nil {
			writeDBError(w, err.Error())
			return
		}
		cfg.AutoSnapshot.Selection = selection
		cfg.AutoSnapshot.Enabled = body.Enabled
		cfg.AutoSnapshot.Every = body.Every
		cfg.AutoSnapshot.Keep = body.Keep
		cfg.AutoSnapshot.KeepFor = body.KeepFor
		if err := config.SaveGlobal(cfg); err != nil {
			writeDBError(w, err.Error())
			return
		}
		writeJSON(w, autoSnapshotStatus())
	default:
		http.NotFound(w, r)
	}
}

// handleAutoSnapshotSite sets one site's tri-state override.
func handleAutoSnapshotSite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/auto-snapshot/"), "/") != "site" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Site string `json:"site"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Site == "" {
		writeDBError(w, "a site and a mode are required")
		return
	}
	if err := config.SetSiteAutoSnapshot(body.Site, body.Mode); err != nil {
		writeDBError(w, err.Error())
		return
	}
	writeJSON(w, autoSnapshotStatus())
}

// autoSnapshotStatus builds the policy payload, dating each site's last and next
// snapshot off the stored snapshots themselves.
func autoSnapshotStatus() autoSnapshotResponse {
	cfg, _ := config.LoadGlobal()
	out := autoSnapshotResponse{
		Enabled:   cfg.AutoSnapshotEnabled(),
		Every:     cfg.AutoSnapshotEvery().String(),
		Keep:      cfg.AutoSnapshotKeep(),
		Selection: cfg.AutoSnapshotSelection(),
		Sites:     []autoSnapshotSiteStatus{},
	}
	if keepFor := cfg.AutoSnapshotKeepFor(); keepFor > 0 {
		out.KeepFor = keepFor.String()
	}
	every := cfg.AutoSnapshotEvery()
	for _, t := range config.AutoSnapshotSiteTargets() {
		row := autoSnapshotSiteStatus{
			Site:     t.Site,
			Domain:   t.Domain,
			Service:  t.Service,
			Database: t.Database,
			Mode:     t.Mode,
			Covered:  cfg.AutoSnapshotModeCovers(t.Mode),
		}
		if last := lastAutoSnapshotAt(t.Service, t.Database); !last.IsZero() {
			next := last.Add(every)
			row.Last, row.Next = &last, &next
		}
		out.Sites = append(out.Sites, row)
	}
	return out
}

// lastAutoSnapshotAt reports when the schedule last snapshotted a database, read
// off the snapshots themselves so it stays true across a watcher restart.
func lastAutoSnapshotAt(service, database string) time.Time {
	snaps, err := serviceops.ListSnapshots(service, database, false)
	if err != nil {
		return time.Time{}
	}
	for _, s := range snaps { // newest first
		if s.Auto {
			return s.Created
		}
	}
	return time.Time{}
}
