package ui

import (
	"strings"

	"github.com/geodro/lerd/internal/podman"
)

// phpImageState is what the status shows for a PHP version's build: the patch
// it runs and whether a newer one is published.
type phpImageState struct {
	Patch  string
	Update bool
}

// Seams so the cached answer can be tested without podman or the registry.
var (
	phpStateFn       = readPHPState
	refreshPHPBaseFn = func(version string) any { return podman.RefreshBaseImageFreshness(version) }
)

// readPHPState asks the host build under the native runtime, where the patch
// is the build on disk, and the image and its base otherwise.
func readPHPState(version string, native bool) phpImageState {
	if native {
		patch, update := nativePHPStatusFor(nativePins(), version)
		return phpImageState{Patch: patch, Update: update}
	}
	st := phpImageState{Patch: podman.FPMPHPVersion(version)}
	if base := podman.BaseImageFreshness(version); base != nil {
		st.Update = base.Stale
	}
	return st
}

// phpState is a PHP version's state from lerd-ui's hourly cache, keyed by its
// FPM unit, so a rebuild restarting that unit drops it.
func phpState(version string, native bool) phpImageState {
	key := "lerd-php" + strings.ReplaceAll(version, ".", "") + "-fpm"
	return cachedUpdate(updates, key, func() phpImageState { return phpStateFn(version, native) })
}
