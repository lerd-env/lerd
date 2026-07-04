package reqstats

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Save writes the current snapshot of every site to path as JSON, via a temp
// file and rename so a reader never sees a half-written file. The watcher calls
// this on its tick; lerd-ui reads it with Load.
func (a *Aggregator) Save(path string) error {
	snap := a.Snapshot()
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a snapshot previously written by Save. A missing or unreadable file
// yields an empty slice, so a UI that starts before the watcher just shows
// nothing rather than erroring.
func Load(path string) []SiteStats {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var snap []SiteStats
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil
	}
	return snap
}

// LoadSite returns the snapshot for one site from the persisted file, ok=false
// when the site has no recorded traffic.
func LoadSite(path, site string) (SiteStats, bool) {
	for _, s := range Load(path) {
		if s.Site == site {
			return s, true
		}
	}
	return SiteStats{}, false
}
