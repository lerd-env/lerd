package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

// autoSnapshotPolicy is what the db auto action reports: the global schedule and
// every site's standing with it.
type autoSnapshotPolicy struct {
	Enabled bool   `json:"enabled"`
	Every   string `json:"every"`
	Keep    int    `json:"keep"`
	KeepFor string `json:"keep_for,omitempty"`
	// Selection is "opt_out" (every site until one is excluded) or "opt_in"
	// (none until a site is included).
	Selection string               `json:"selection"`
	Sites     []autoSnapshotSiteMC `json:"sites"`
}

type autoSnapshotSiteMC struct {
	Site     string     `json:"site"`
	Service  string     `json:"service"`
	Database string     `json:"database"`
	Mode     string     `json:"mode"`
	Covered  bool       `json:"covered"`
	Last     *time.Time `json:"last,omitempty"`
}

func execDBAuto(_ map[string]any) (any, *rpcError) {
	data, _ := json.MarshalIndent(readAutoSnapshotPolicy(), "", "  ")
	return toolOK(string(data)), nil
}

// execDBAutoSet writes the policy, or one site's override when site is given.
func execDBAutoSet(args map[string]any) (any, *rpcError) {
	if site := strArg(args, "site"); site != "" {
		mode := strArg(args, "mode")
		if err := config.SetSiteAutoSnapshot(site, mode); err != nil {
			return toolErr(err.Error()), nil
		}
		return toolOK(fmt.Sprintf("Automatic snapshots for %s: %s", site, autoSnapshotModeWord(mode))), nil
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return toolErr(err.Error()), nil
	}
	if _, ok := args["enabled"]; ok {
		cfg.AutoSnapshot.Enabled = boolArg(args, "enabled")
	}
	if every := strArg(args, "every"); every != "" {
		if _, err := time.ParseDuration(every); err != nil {
			return toolErr(fmt.Sprintf("every %q is not a duration, try 6h or 24h", every)), nil
		}
		cfg.AutoSnapshot.Every = every
	}
	if keepFor := strArg(args, "keep_for"); keepFor != "" {
		if _, err := time.ParseDuration(keepFor); err != nil {
			return toolErr(fmt.Sprintf("keep_for %q is not a duration, try 168h", keepFor)), nil
		}
		cfg.AutoSnapshot.KeepFor = keepFor
	}
	if selection := strArg(args, "selection"); selection != "" {
		clean, err := config.NormalizeAutoSnapshotSelection(selection)
		if err != nil {
			return toolErr(err.Error()), nil
		}
		cfg.AutoSnapshot.Selection = clean
	}
	if keep, ok := args["keep"]; ok {
		if n, fine := keep.(float64); fine {
			cfg.AutoSnapshot.Keep = int(n)
		}
	}
	if err := config.SaveGlobal(cfg); err != nil {
		return toolErr(err.Error()), nil
	}
	data, _ := json.MarshalIndent(readAutoSnapshotPolicy(), "", "  ")
	return toolOK(string(data)), nil
}

// execDBSnapshotKeep pins an automatic snapshot so retention leaves it alone, or
// releases it back under retention with kept:false.
func execDBSnapshotKeep(args map[string]any) (any, *rpcError) {
	name := strArg(args, "name")
	if name == "" {
		return toolErr("name is required"), nil
	}
	target, err := mcpSnapshotTarget(args)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	kept := true
	if _, ok := args["kept"]; ok {
		kept = boolArg(args, "kept")
	}
	if err := serviceops.SetSnapshotKept(target.Service, target.Database, name, target.AllDatabases, kept); err != nil {
		return toolErr(err.Error()), nil
	}
	if kept {
		return toolOK(fmt.Sprintf("Snapshot %q will be kept, retention leaves it alone", name)), nil
	}
	return toolOK(fmt.Sprintf("Snapshot %q is back under retention", name)), nil
}

func readAutoSnapshotPolicy() autoSnapshotPolicy {
	cfg, _ := config.LoadGlobal()
	out := autoSnapshotPolicy{
		Enabled:   cfg.AutoSnapshotEnabled(),
		Every:     cfg.AutoSnapshotEvery().String(),
		Keep:      cfg.AutoSnapshotKeep(),
		Selection: cfg.AutoSnapshotSelection(),
		Sites:     []autoSnapshotSiteMC{},
	}
	if keepFor := cfg.AutoSnapshotKeepFor(); keepFor > 0 {
		out.KeepFor = keepFor.String()
	}
	for _, t := range config.AutoSnapshotSiteTargets() {
		row := autoSnapshotSiteMC{
			Site:     t.Site,
			Service:  t.Service,
			Database: t.Database,
			Mode:     t.Mode,
			Covered:  cfg.AutoSnapshotModeCovers(t.Mode),
		}
		if last := lastAutoSnapshotFor(t.Service, t.Database); !last.IsZero() {
			row.Last = &last
		}
		out.Sites = append(out.Sites, row)
	}
	return out
}

func lastAutoSnapshotFor(service, database string) time.Time {
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

func autoSnapshotModeWord(mode string) string {
	switch mode {
	case config.AutoSnapshotOn:
		return "always"
	case config.AutoSnapshotOff:
		return "never"
	}
	return "following the global policy"
}

// mcpSnapshot is a stored snapshot plus when retention will drop it, so an
// assistant reading the list can say what is about to disappear.
type mcpSnapshot struct {
	serviceops.Snapshot
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RunsLeft  int        `json:"runs_left,omitempty"`
	Estimated bool       `json:"estimated,omitempty"`
}

func snapshotsWithRetention(snaps []serviceops.Snapshot) []mcpSnapshot {
	cfg, _ := config.LoadGlobal()
	expiries := serviceops.SnapshotExpiries(serviceops.RetentionPolicy{
		Keep:    cfg.AutoSnapshotKeep(),
		KeepFor: cfg.AutoSnapshotKeepFor(),
		Every:   cfg.AutoSnapshotEvery(),
	}, snaps)

	out := make([]mcpSnapshot, 0, len(snaps))
	for i, snap := range snaps {
		row := mcpSnapshot{Snapshot: snap, RunsLeft: expiries[i].RunsLeft, Estimated: expiries[i].Estimated}
		if !expiries[i].At.IsZero() {
			at := expiries[i].At
			row.ExpiresAt = &at
		}
		out = append(out, row)
	}
	return out
}
