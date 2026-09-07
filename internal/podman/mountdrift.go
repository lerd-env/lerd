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
	return containerMountField(name, "Source")
}

// containerMountField is containerMounts for one field of the mount list, so a
// check can ask by destination where asking by source would answer wrongly.
func containerMountField(name, field string) (bool, []string) {
	out, err := Run("inspect", "--format={{.State.Running}}#{{range .Mounts}}{{."+field+"}}|{{end}}", name)
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

// composerMountTarget is where the FPM quadlet puts lerd's own composer, over
// the copy the image was built with.
const composerMountTarget = "/usr/local/bin/composer"

// UnitMissingComposerMount reports whether a running container still carries the
// image's composer where its quadlet now mounts lerd's. It asks by destination
// because the whole-home mount already covers the phar's source path, so the
// source-based check would call this one present on a container that has never
// had it.
func UnitMissingComposerMount(unit string) bool {
	running, destinations := containerMountField(unit, "Destination")
	if !running {
		return false
	}
	for _, d := range destinations {
		if d == composerMountTarget {
			return false
		}
	}
	return true
}
