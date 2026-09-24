package stats

import "strings"

// WithoutSites drops the containers that belong to the named sites, so streaming
// mode can keep a private site's name out of the resource figures. A site's
// containers are named lerd-<worker>-<site>, with -<worktree> for a worktree.
func WithoutSites(snap Snapshot, sites map[string]bool) Snapshot {
	if len(sites) == 0 {
		return snap
	}
	kept := make([]ContainerStat, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		if !belongsToSite(c.Name, sites) {
			kept = append(kept, c)
		}
	}
	snap.Containers = kept
	return snap
}

func belongsToSite(name string, sites map[string]bool) bool {
	for site := range sites {
		if strings.HasSuffix(name, "-"+site) || strings.Contains(name, "-"+site+"-") {
			return true
		}
	}
	return false
}
