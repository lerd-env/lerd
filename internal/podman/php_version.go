package podman

import (
	"strings"
	"sync"
)

// The full PHP version (e.g. "8.5.1") is not something status can read cheaply:
// it lives inside the image. We probe it once per image build with a throwaway
// container and cache it, keyed on the image's containerfile hash so a rebuild
// re-probes. buildStatus reads the cache only — the probe runs in the
// background — so the status hot path never blocks on podman.
var (
	phpVerMu       sync.Mutex
	phpVerCache    = map[string]phpVerEntry{}
	phpVerInflight = map[string]bool{}
)

type phpVerEntry struct{ hash, patch string }

// FPMPHPVersion returns the cached full PHP version for a version's FPM image,
// or "" when it has not been probed yet or the image is not built. It never
// touches podman on the caller's path; it schedules a background probe that
// fills the cache for a later read and refreshes it after a rebuild.
func FPMPHPVersion(version string) string {
	phpVerMu.Lock()
	patch := phpVerCache[version].patch
	phpVerMu.Unlock()
	go refreshPHPVersion(version)
	return patch
}

// refreshPHPVersion probes the image's PHP version and caches it, unless the
// cache is already fresh for the current image hash. Safe to call concurrently:
// one probe per version runs at a time.
func refreshPHPVersion(version string) {
	phpVerMu.Lock()
	if phpVerInflight[version] {
		phpVerMu.Unlock()
		return
	}
	phpVerInflight[version] = true
	phpVerMu.Unlock()
	defer func() {
		phpVerMu.Lock()
		phpVerInflight[version] = false
		phpVerMu.Unlock()
	}()

	hash := imageLabelFn(FPMImageName(version), fpmContainerfileHashLabel)
	if hash == "" {
		return // image not built (or predates the label)
	}
	phpVerMu.Lock()
	cur, ok := phpVerCache[version]
	phpVerMu.Unlock()
	if ok && cur.hash == hash {
		return // already fresh for this image
	}

	out, err := execCommand(PodmanBin(), "run", "--rm", FPMImageName(version), "php", "-r", "echo PHP_VERSION;").Output()
	if err != nil {
		return
	}
	patch := strings.TrimSpace(string(out))
	if patch == "" {
		return
	}
	phpVerMu.Lock()
	phpVerCache[version] = phpVerEntry{hash: hash, patch: patch}
	phpVerMu.Unlock()
}
