package podman

import (
	"strings"
	"sync"
)

// The full PHP version (e.g. "8.5.1") is not something status can read cheaply:
// it lives inside the image. We probe it once per image with a throwaway
// container and cache it, keyed on the image ID so every rebuild re-probes —
// including a base-image update, which ships a new PHP patch without touching
// the containerfile. buildStatus reads the cache only — the probe runs in the
// background — so the status hot path never blocks on podman.
var (
	phpVerMu     sync.Mutex
	phpVerCache  = map[string]phpVerEntry{}
	phpVerProbes = map[string]*sync.Mutex{}
	imageIDFn    = FPMImageID
)

type phpVerEntry struct{ imageID, patch string }

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

// RefreshFPMPHPVersion probes synchronously, waiting for a probe already in
// flight rather than skipping it. A caller that has just rebuilt the image uses
// it to get the new patch into the cache before it reports the build done, so
// the status that follows carries the number rather than nothing.
func RefreshFPMPHPVersion(version string) {
	mu := phpVerProbeMu(version)
	mu.Lock()
	defer mu.Unlock()
	probePHPVersion(version)
}

// refreshPHPVersion is the background path: it gives up when another probe for
// the version is running, since that one fills the same cache entry.
func refreshPHPVersion(version string) {
	mu := phpVerProbeMu(version)
	if !mu.TryLock() {
		return
	}
	defer mu.Unlock()
	probePHPVersion(version)
}

func phpVerProbeMu(version string) *sync.Mutex {
	phpVerMu.Lock()
	defer phpVerMu.Unlock()
	mu, ok := phpVerProbes[version]
	if !ok {
		mu = &sync.Mutex{}
		phpVerProbes[version] = mu
	}
	return mu
}

// probePHPVersion reads the PHP version out of the image and caches it, unless
// the cache already holds it for the image that is there now.
func probePHPVersion(version string) {
	id := imageIDFn(version)
	if id == "" {
		return // image not built
	}
	phpVerMu.Lock()
	cur, ok := phpVerCache[version]
	phpVerMu.Unlock()
	if ok && cur.imageID == id {
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
	phpVerCache[version] = phpVerEntry{imageID: id, patch: patch}
	phpVerMu.Unlock()
	if cur.patch != patch {
		announceBackgroundRead(fpmUnit(version))
	}
}

func fpmUnit(version string) string {
	return "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
}
