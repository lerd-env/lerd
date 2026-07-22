package podman

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// MountRepair records one stale bind mount dropped from a quadlet.
type MountRepair struct {
	Unit string
	Path string
	Site string // site the path belongs to, empty when it maps to none
}

// PruneMissingVolumes removes the Volume= lines that would make podman refuse
// to start the container: a host path bind-mounted at the same location inside
// it that no longer exists on disk, which aborts the start with "statfs <path>:
// no such file or directory" and takes every site down with it (#1083). Only
// self-mounts of an absolute host path are considered, so the quadlet's own
// config, socket and ini mounts (which lerd creates on demand) are left alone.
// It returns the rewritten content and the paths that were dropped.
func PruneMissingVolumes(content string) (string, []string) {
	var removed []string
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if src, ok := staleSelfMount(line); ok {
			removed = append(removed, src)
			continue
		}
		kept = append(kept, line)
	}
	if len(removed) == 0 {
		return content, nil
	}
	return strings.Join(kept, "\n"), removed
}

// staleSelfMount reports whether a quadlet line is a Volume=path:path mount
// whose host source has gone missing, returning that source.
func staleSelfMount(line string) (string, bool) {
	spec, ok := strings.CutPrefix(strings.TrimSpace(line), "Volume=")
	if !ok {
		return "", false
	}
	src, rest, found := strings.Cut(spec, ":")
	if !found || !bindMountable(src) || strings.Contains(src, "%") {
		return "", false
	}
	dst, _, _ := strings.Cut(rest, ":")
	if dst != src {
		return "", false
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		return "", false
	}
	return src, true
}

// RepairMissingMounts drops stale bind mounts from every quadlet on disk before
// the units are started, and reports what was removed so the caller can name the
// project responsible. Containers already running keep their mounts until their
// next restart, which is when the quadlet is read again.
func RepairMissingMounts() []MountRepair {
	dir := config.QuadletDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var repairs []MountRepair
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".container") {
			continue
		}
		path := filepath.Join(dir, name)
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		pruned, removed := PruneMissingVolumes(string(existing))
		if len(removed) == 0 {
			continue
		}
		if writeErr := os.WriteFile(path, []byte(pruned), 0644); writeErr != nil {
			continue
		}
		unit := strings.TrimSuffix(name, ".container")
		for _, p := range removed {
			repairs = append(repairs, MountRepair{Unit: unit, Path: p, Site: siteForPath(p)})
		}
	}
	if len(repairs) > 0 {
		_ = DaemonReloadFn()
	}
	return repairs
}

// siteForPath returns the name of the site the path belongs to, matching the
// site root itself or any directory under it.
func siteForPath(path string) string {
	reg, err := config.LoadSites()
	if err != nil || reg == nil {
		return ""
	}
	best := ""
	bestLen := 0
	for _, site := range reg.Sites {
		root := filepath.Clean(site.Path)
		if root == "" || root == "." {
			continue
		}
		if path != root && !strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/") {
			continue
		}
		if len(root) > bestLen {
			best, bestLen = site.Name, len(root)
		}
	}
	return best
}
