package podman

import (
	"path/filepath"
	"strings"
)

// containerMounts returns whether the named container is running and the
// host-side sources of the mounts it is running with. Writing a quadlet does
// not touch a running container, so this is the only honest answer to "can it
// reach this path".
func containerMounts(name string) (bool, []string) {
	out, err := Run("inspect", "--format={{.State.Running}}#{{range .Mounts}}{{.Source}}|{{end}}", name)
	if err != nil {
		return false, nil
	}
	state, mounts, _ := strings.Cut(strings.TrimSpace(out), "#")
	if state != "true" {
		return false, nil
	}
	var sources []string
	for _, s := range strings.Split(mounts, "|") {
		if s = strings.TrimSpace(s); s != "" {
			sources = append(sources, s)
		}
	}
	return true, sources
}

// UnitMissingMounts reports whether the container behind a unit is running
// without a bind mount covering one of the given paths, i.e. its quadlet has
// been updated underneath it and only a restart will make the paths reachable
// (issue #914). A stopped or unknown container never counts: it picks the
// quadlet up when it next starts.
func UnitMissingMounts(unit string, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	running, sources := containerMounts(unit)
	if !running {
		return false
	}
	for _, p := range paths {
		if !mountCovers(sources, p) {
			return true
		}
	}
	return false
}

// mountCovers reports whether one of the mount sources is the path itself or
// one of its ancestors. Podman may report a source in resolved form, so a
// symlinked path is compared both ways rather than being reported missing
// forever, which would restart the containers on every call.
func mountCovers(sources []string, path string) bool {
	candidates := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		candidates = append(candidates, resolved)
	}
	for _, src := range sources {
		for _, p := range candidates {
			if p == src || strings.HasPrefix(p, strings.TrimSuffix(src, "/")+"/") {
				return true
			}
		}
	}
	return false
}
