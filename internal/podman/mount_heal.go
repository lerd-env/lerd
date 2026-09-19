package podman

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// StaleQuadletMounts returns the host paths a written quadlet still bind-mounts
// that are no longer on disk, across every installed PHP version.
//
// Podman refuses to start a container whose bind source is missing, so one such
// path takes every site on that version down with exit 125 and a restart loop
// that says nothing about the directory that went. The paths are dropped when a
// quadlet is written, but a reboot starts the containers from the file as it
// stands, and the directory a registered ODBC driver lives in is exactly the
// kind that disappears between one boot and the next: a vendor client gets
// uninstalled, /opt gets tidied, an external disk is not plugged in.
func StaleQuadletMounts() []string {
	versions, _ := listInstalledPHPVersions()
	seen := map[string]bool{}
	var stale []string
	for _, v := range versions {
		content, err := os.ReadFile(filepath.Join(config.QuadletDir(), SharedFPMContainerName(v)+".container"))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			src, ok := quadletVolumeSource(line)
			if !ok || seen[src] {
				continue
			}
			if _, err := os.Stat(src); err != nil {
				seen[src] = true
				stale = append(stale, src)
			}
		}
	}
	return stale
}

// quadletVolumeSource picks the host path out of a Volume= line, skipping the
// named volumes and the %h specifier, which are not paths to check.
func quadletVolumeSource(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "Volume=")
	if !ok {
		return "", false
	}
	src, _, ok := strings.Cut(rest, ":")
	if !ok || src == "" || !strings.HasPrefix(src, "/") {
		return "", false
	}
	return src, true
}

// HealStaleQuadletMounts rewrites the PHP quadlets when one of them still names
// a host path that has gone, so the next restart attempt starts from a file
// podman will accept. Reports whether anything was rewritten.
func HealStaleQuadletMounts() (bool, error) {
	if len(StaleQuadletMounts()) == 0 {
		return false, nil
	}
	return true, RewriteFPMQuadlets()
}
