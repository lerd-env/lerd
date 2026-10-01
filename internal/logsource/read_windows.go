//go:build windows

package logsource

import "github.com/geodro/lerd/internal/unitlog"

// readJournal has no journald on Windows. Container units are read with
// `podman logs`; host-supervised ones tail their file under the data dir.
func readJournal(src Source, opts Opts) (Result, error) {
	if unitlog.IsContainerUnit(src.Locator) {
		podSrc := src
		podSrc.Kind = KindPodman
		return readPodman(podSrc, opts)
	}
	fileSrc := Source{
		Name:    src.Name,
		Kind:    KindFile,
		Locator: unitlog.LogPath(src.Locator),
		Scope:   src.Scope,
		Format:  "raw",
	}
	return readFile(fileSrc, opts)
}
