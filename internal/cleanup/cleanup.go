// Package cleanup reclaims podman disk lerd's own image rebuilds leave behind.
// The safe tier only ever removes what is provably lerd's: an image with a
// dev.lerd.* label, or the lerd-php*-fpm-base repo name only lerd's pre-built
// base images use. The deep tier additionally reaps every dangling image, which
// is untagged and unreferenced by definition so removing it strands nothing, and
// catalog service images no service references any more. Neither tier ever
// touches a tagged image in use, a container, or a named volume.
package cleanup

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/geodro/lerd/internal/podman"
)

// lerdLabelPrefix is stamped (as a hash label) on every image lerd builds. Its
// presence is the single proof that an image is lerd's, and the only gate for
// considering a built image for removal.
const lerdLabelPrefix = "dev.lerd."

// baseImageRe matches a lerd pre-built PHP base image ref. These are pulled from
// a registry and carry no dev.lerd.* label, so the repo name (which only lerd's
// base images use) is the ownership signal.
var baseImageRe = regexp.MustCompile(`/lerd-php\d+-fpm-base:`)

// Target is one reclaimable resource.
type Target struct {
	Kind  string // "image"
	ID    string // short image ID
	Desc  string // human description for the plan output
	Bytes int64
}

// Plan is the set of lerd-owned resources that are safe to reclaim.
type Plan struct {
	Targets []Target
	// Held counts dangling images a running container still holds that would
	// otherwise be reaped. They can't be removed now, but a restart recreates the
	// container on the current image and releases them, so the caller can hint it.
	Held HeldByContainers
}

// HeldByContainers tallies reclaimable disk a restart would free.
type HeldByContainers struct {
	Count int
	Bytes int64
}

// ReclaimBytes is the total disk the plan would free.
func (p Plan) ReclaimBytes() int64 {
	var total int64
	for _, t := range p.Targets {
		total += t.Bytes
	}
	return total
}

// image mirrors the fields lerd needs from `podman images --format json`.
// SharedSize is the bytes of layers this image shares with others; subtracting
// it from Size gives the disk actually freed by removing the image, since shared
// base layers stay behind for the live images that still reference them.
type image struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names"`
	Size       int64             `json:"Size"`
	SharedSize int64             `json:"SharedSize"`
	Labels     map[string]string `json:"Labels"`
	// Containers is how many containers still reference this image. Any non-zero
	// count means podman refuses to remove it, so it must never be listed.
	Containers int `json:"Containers"`
}

// inUse reports whether a container still holds the image, which makes it
// unremovable: podman errors "image is in use by a container". Listing such an
// image would show it as reclaimable forever while freeing nothing.
func inUse(img image) bool { return img.Containers > 0 }

// reclaimable is the disk removing img would actually free: its layers that no
// other image shares. Never negative.
func reclaimable(img image) int64 {
	if u := img.Size - img.SharedSize; u > 0 {
		return u
	}
	return 0
}

// scanImages and imageLayers are the seams tests override; they read local
// image state via podman. imageLayers inspects in one batched call, keyed by the
// IDs passed in (podman returns one result line per arg, in order).
var (
	scanImages  = podmanImages
	imageLayers = podmanImageLayers
)

func podmanImages() ([]image, error) {
	out, err := podman.Run("images", "--format", "json")
	if err != nil {
		return nil, err
	}
	var imgs []image
	if err := json.Unmarshal([]byte(out), &imgs); err != nil {
		return nil, fmt.Errorf("parsing podman images: %w", err)
	}
	return imgs, nil
}

func podmanImageLayers(ids []string) (map[string][]string, error) {
	if len(ids) == 0 {
		return map[string][]string{}, nil
	}
	args := append([]string{"image", "inspect", "--format", "{{json .RootFS.Layers}}"}, ids...)
	out, err := podman.Run(args...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != len(ids) {
		return nil, fmt.Errorf("layer inspect: %d results for %d ids", len(lines), len(ids))
	}
	m := make(map[string][]string, len(ids))
	for i, line := range lines {
		var layers []string
		if err := json.Unmarshal([]byte(line), &layers); err != nil {
			return nil, fmt.Errorf("parsing layers for %s: %w", ids[i], err)
		}
		m[ids[i]] = layers
	}
	return m, nil
}

// Inspect returns the cleanup plan. The always-safe tier reclaims what is
// provably lerd's:
//   - derived images lerd built and then orphaned: lerd tags every live build
//     with a fixed :local tag, so a rebuild re-points the tag and leaves the old
//     image dangling — the unambiguous "superseded" signal.
//   - pre-built base images nothing live is built on: a base for an old
//     Containerfile hash, or for a PHP version no longer installed.
//
// Both removals are refcount-safe: layers a live image still shares are kept.
//
// When deep is true it also reclaims the aggressive tier: every remaining
// dangling image (untagged and unreferenced, so removing it strands nothing),
// plus catalog service images no service references any more (see deepTargets).
// An image a container still holds is skipped everywhere, since podman can't
// remove it. If the protected set can't be built the service reap is skipped
// rather than risk a wrong removal.
func Inspect(deep bool) (Plan, error) {
	imgs, err := scanImages()
	if err != nil {
		return Plan{}, err
	}

	var p Plan
	add := func(id, desc string, bytes int64) {
		p.Targets = append(p.Targets, Target{Kind: "image", ID: id, Desc: desc, Bytes: bytes})
	}
	var baseCandidates []image
	for _, img := range imgs {
		switch {
		case inUse(img):
			// A container still holds this image; podman can't remove it, so skip
			// it. If it is a dangling image this tier would otherwise reap (an old
			// build the container hasn't been recreated off yet), tally it so the
			// caller can hint that a restart would release the space.
			if isOrphaned(img) && (isLerd(img) || deep) {
				p.Held.Count++
				p.Held.Bytes += reclaimable(img)
			}
			continue
		case isOrphaned(img):
			// A dangling image is untagged and unreferenced, so removing it frees
			// disk and strands nothing. Provable lerd orphans go in every tier;
			// other dangling leftovers only when the deep tier is on.
			if isLerd(img) || deep {
				add(shortID(img.ID), describeOrphan(img), reclaimable(img))
			}
		default:
			if baseName(img) != "" {
				baseCandidates = append(baseCandidates, img)
			}
		}
	}

	// Only inspect layers when there is a base image whose in-use status must be
	// decided; with none, skip the podman inspect calls entirely. If either the
	// live-image set or the base layers can't be fully read, keep every base
	// (an inspect failure must never make an in-use base look orphaned).
	if len(baseCandidates) > 0 {
		if live, ok := liveLayers(imgs); ok {
			if baseLayers, err := imageLayers(imageIDs(baseCandidates)); err == nil {
				for _, img := range baseCandidates {
					if !builtUpon(baseLayers[img.ID], live) {
						add(baseName(img), "orphaned PHP base image", reclaimable(img))
					}
				}
			}
		}
	}

	if deep {
		repos, repoErr := serviceRepos()
		prot, protErr := protectedImages()
		if repoErr == nil && protErr == nil {
			p.Targets = append(p.Targets, deepTargets(imgs, repos, prot)...)
		}
	}
	// podman lists a multi-tag image once per tag, so dedupe by ref/ID to avoid
	// counting or trying to remove the same target twice.
	p.Targets = dedupTargets(p.Targets)
	return p, nil
}

// dedupTargets drops repeated targets, keeping the first of each ID/ref.
func dedupTargets(ts []Target) []Target {
	seen := make(map[string]bool, len(ts))
	out := make([]Target, 0, len(ts))
	for _, t := range ts {
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		out = append(out, t)
	}
	return out
}

// imageIDs returns the IDs of the given images, for a batched layer inspect.
func imageIDs(imgs []image) []string {
	ids := make([]string, len(imgs))
	for i, img := range imgs {
		ids[i] = img.ID
	}
	return ids
}

// liveLayers collects every layer belonging to a live (still-tagged) lerd-built
// image, so a base image can be told apart from one nothing is built on. The
// bool is false when any live image's layers couldn't be read, so the caller
// can refuse to reap bases against an incomplete live set.
func liveLayers(imgs []image) (map[string]bool, bool) {
	var ids []string
	for _, img := range imgs {
		if isLerd(img) && !isOrphaned(img) {
			ids = append(ids, img.ID)
		}
	}
	set := map[string]bool{}
	if len(ids) == 0 {
		return set, true
	}
	byID, err := imageLayers(ids)
	if err != nil {
		return set, false
	}
	for _, layers := range byID {
		for _, l := range layers {
			set[l] = true
		}
	}
	return set, true
}

// builtUpon reports whether a live image is built on a base, given the base's
// layers: true when the base's top layer appears in some live image. Unknown
// (empty) layers return true, so a base we cannot place is kept.
func builtUpon(layers []string, live map[string]bool) bool {
	if len(layers) == 0 {
		return true
	}
	return live[layers[len(layers)-1]]
}

// baseName returns the image's lerd base-image ref, or "" if it isn't one.
func baseName(img image) string {
	for _, n := range img.Names {
		if baseImageRe.MatchString(n) {
			return n
		}
	}
	return ""
}

// removeImage is the seam tests override; it deletes one image by ID.
var removeImage = podmanRemoveImage

func podmanRemoveImage(id string) error {
	return podman.RunSilent("image", "rm", id)
}

// Apply removes every target in the plan and returns how many images were
// actually removed and the disk reclaimed (a skipped target counts toward
// neither). It sweeps in repeated passes, retrying targets that failed until a full pass
// frees nothing new: podman refuses a parent image while a child is still
// present, so removing a dangling build chain listed parent-first needs the
// children gone first. A target that never succeeds (e.g. it became referenced
// since Inspect) is simply left, so one stuck image can't abort the sweep.
func Apply(p Plan) (removed int, reclaimed int64) {
	remaining := p.Targets
	for len(remaining) > 0 {
		var stuck []Target
		progress := false
		for _, t := range remaining {
			if err := removeImage(t.ID); err != nil {
				stuck = append(stuck, t)
				continue
			}
			removed++
			reclaimed += t.Bytes
			progress = true
		}
		if !progress {
			break
		}
		remaining = stuck
	}
	return removed, reclaimed
}

// isLerd reports whether the image was built by lerd, proven by a dev.lerd.*
// label. Only images that pass this are ever eligible for removal.
func isLerd(img image) bool {
	for k := range img.Labels {
		if strings.HasPrefix(k, lerdLabelPrefix) {
			return true
		}
	}
	return false
}

// isOrphaned reports whether a lerd image has lost its tag (no names, or only
// the placeholder <none>:<none>), meaning a newer build superseded it.
func isOrphaned(img image) bool {
	for _, n := range img.Names {
		if n != "" && n != "<none>:<none>" {
			return false
		}
	}
	return true
}

// describeOrphan names a dangling image for the plan: its lerd build kind when
// the image is labelled, or a generic dangling leftover otherwise.
func describeOrphan(img image) string {
	if isLerd(img) {
		return describe(img.Labels)
	}
	return "dangling image"
}

// describe names the kind of orphaned derived image for the plan output, by its
// build label (FPM or FrankenPHP). Pre-built base images are described
// separately at their call site.
func describe(labels map[string]string) string {
	for k := range labels {
		if strings.Contains(k, "frankenphp") {
			return "orphaned FrankenPHP image"
		}
	}
	return "orphaned PHP image"
}

// shortID trims a sha256: prefix and truncates to podman's 12-char short form.
func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
