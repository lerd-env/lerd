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
	return SaveSnapshot(a.Snapshot(), path)
}

// SaveSnapshot persists an already-computed snapshot, so a caller that also needs
// the snapshot (e.g. to feed slow-route notifications) can build it once and
// avoid recomputing it inside Save.
func SaveSnapshot(snap []SiteStats, path string) error {
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

// RemoveSite drops a site's snapshot from the persisted file, covering both the
// bare site key and its "<site>/<branch>" worktree keys, so an unlinked site
// stops lingering in the stats file. A missing file is a no-op.
func RemoveSite(path, site string) error {
	snap := Load(path)
	if snap == nil {
		return nil
	}
	kept := snap[:0]
	for _, s := range snap {
		if !KeyBelongsTo(s.Site, site) {
			kept = append(kept, s)
		}
	}
	if len(kept) == len(snap) {
		return nil
	}
	return SaveSnapshot(kept, path)
}
