package podman

import (
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/origin"
	"github.com/geodro/lerd/internal/registry"
)

// fpmBaseDigestLabel records the registry digest of the prebuilt base an FPM
// image was built from. The base tag is the recipe hash, so an upstream PHP
// refresh republishes the same tag with new content and no local hash moves:
// this label is the only thing on the machine that can notice.
const fpmBaseDigestLabel = "dev.lerd.fpm.base-digest"

// Seams for the freshness check so tests can fake the registry.
var (
	cachedManifestDigestFn  = registry.CachedManifestDigest
	manifestDigestFn        = registry.ManifestDigest
	refreshManifestDigestFn = registry.RefreshManifestDigest
)

// BaseImageStatus reports a PHP version's image against the prebuilt base it
// was built from. Stale means the base has been republished since, so a
// rebuild would pick up whatever upstream shipped (a PHP or Alpine fix).
type BaseImageStatus struct {
	Version      string `json:"version"`
	Ref          string `json:"ref,omitempty"`
	BuiltDigest  string `json:"built_digest,omitempty"`
	LatestDigest string `json:"latest_digest,omitempty"`
	Stale        bool   `json:"stale"`
}

// baseImageRefs returns the GHCR refs of the prebuilt base for version under
// the current recipe hash, i.e. what a build would pull right now.
func baseImageRefs(version string) []string {
	hash, err := baseContainerfileHash()
	if err != nil {
		return nil
	}
	return origin.BaseImageRefs(strings.ReplaceAll(version, ".", ""), hash)
}

// BaseImageFreshness reports whether version's image is behind its prebuilt
// base, answering from the digest cache only. A cold entry returns nil and
// schedules the registry read in the background, so snapshot rebuilds never
// wait on the network. nil also means "nothing to say": no image, no recorded
// base (a local build), or no digest for the tag.
func BaseImageFreshness(version string) *BaseImageStatus {
	built := recordedBaseDigest(version)
	if built == "" {
		return nil
	}
	for _, ref := range baseImageRefs(version) {
		if latest, ok := cachedManifestDigestFn(ref); ok {
			return baseImageStatus(version, ref, built, latest)
		}
	}
	scheduleBaseDigestRefresh(version)
	return nil
}

// CheckBaseImageFreshness answers from the digest cache, reading through to
// the registry when nothing is cached. Blocking, for one-shot callers like
// doctor that have no later snapshot to pick up a background refresh.
func CheckBaseImageFreshness(version string) *BaseImageStatus {
	built := recordedBaseDigest(version)
	if built == "" {
		return nil
	}
	for _, ref := range baseImageRefs(version) {
		if latest, err := manifestDigestFn(ref); err == nil {
			return baseImageStatus(version, ref, built, latest)
		}
	}
	return nil
}

// RefreshBaseImageFreshness re-reads the base digest from the registry,
// bypassing the cache. Blocking, for the manual check-for-updates path and
// doctor. nil when the registry cannot answer, so being offline never reads as
// an update.
func RefreshBaseImageFreshness(version string) *BaseImageStatus {
	built := recordedBaseDigest(version)
	if built == "" {
		return nil
	}
	for _, ref := range baseImageRefs(version) {
		if latest, err := refreshManifestDigestFn(ref); err == nil {
			return baseImageStatus(version, ref, built, latest)
		}
	}
	return nil
}

// baseLabelTTL bounds how long a read of the base-digest label is reused. The
// label only moves when that version's image is rebuilt, which drops the entry
// anyway; the memo is what keeps a status rebuild every 15 seconds from
// forking a podman inspect per installed version.
const baseLabelTTL = time.Minute

type baseLabelEntry struct {
	digest string
	at     time.Time
}

var (
	baseLabelMu   sync.Mutex
	baseLabelMemo = map[string]baseLabelEntry{}
)

// recordedBaseDigest returns the base digest stamped on a version's image.
func recordedBaseDigest(version string) string {
	image := FPMImageName(version)
	baseLabelMu.Lock()
	entry, ok := baseLabelMemo[image]
	baseLabelMu.Unlock()
	if ok && time.Since(entry.at) < baseLabelTTL {
		return entry.digest
	}
	digest := imageLabelFn(image, fpmBaseDigestLabel)
	baseLabelMu.Lock()
	baseLabelMemo[image] = baseLabelEntry{digest: digest, at: time.Now()}
	baseLabelMu.Unlock()
	return digest
}

// forgetRecordedBaseDigest drops the memo for a version, so a rebuild's new
// label is read back immediately rather than after the TTL.
func forgetRecordedBaseDigest(version string) {
	baseLabelMu.Lock()
	delete(baseLabelMemo, FPMImageName(version))
	baseLabelMu.Unlock()
}

func baseImageStatus(version, ref, built, latest string) *BaseImageStatus {
	return &BaseImageStatus{
		Version:      version,
		Ref:          ref,
		BuiltDigest:  built,
		LatestDigest: latest,
		Stale:        !strings.EqualFold(strings.TrimSpace(built), strings.TrimSpace(latest)),
	}
}

var (
	baseRefreshMu  sync.Mutex
	baseRefreshing = map[string]bool{}
)

// scheduleBaseDigestRefresh warms the digest cache for version off the caller's
// goroutine, one refresh per version at a time.
func scheduleBaseDigestRefresh(version string) {
	baseRefreshMu.Lock()
	if baseRefreshing[version] {
		baseRefreshMu.Unlock()
		return
	}
	baseRefreshing[version] = true
	baseRefreshMu.Unlock()

	go func() {
		defer func() {
			baseRefreshMu.Lock()
			delete(baseRefreshing, version)
			baseRefreshMu.Unlock()
		}()
		for _, ref := range baseImageRefs(version) {
			if _, err := refreshManifestDigestFn(ref); err == nil {
				return
			}
		}
	}()
}
