package cleanup

import (
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/imgledger"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/registry"
)

// serviceRepos and protectedImages are the seams tests override. serviceRepos
// is the set of normalized image repos lerd's preset catalog manages (mysql,
// redis, ...); protectedImages is every image a service currently uses or holds
// as a rollback target, which --deep must never remove.
var (
	serviceRepos    = realServiceRepos
	protectedImages = realProtectedImages

	// referencedImages is the seam SweepRefs uses to test a handful of specific
	// refs without materialising the whole protected set: it scans cheap config
	// sources first and stops as soon as every candidate is accounted for.
	referencedImages = realReferencedImages

	// installedServiceImages is a seam so the short-circuit (config covers every
	// candidate, so the per-quadlet image reads are skipped) is assertable.
	installedServiceImages = realInstalledServiceImages

	// loadPulledImages is the seam onto lerd's pull ledger. The managed tier gates
	// the catalog reap on it; the deep tier ignores it and reaps any unused
	// image, a user's own copy included.
	loadPulledImages = imgledger.Load
)

// canonPulled canonicalises the ledger refs so they compare equal to the
// canonicalised image names the reap inspects, regardless of short vs
// fully-qualified form.
func canonPulled() map[string]bool {
	raw := loadPulledImages()
	out := make(map[string]bool, len(raw))
	for r := range raw {
		out[canonRef(r)] = true
	}
	return out
}

// realReferencedImages returns the subset of candidates (keyed by canonical ref)
// still used by a service config entry, a custom service, or an installed service
// quadlet. It scans config (cheap, cached) before the per-quadlet image reads and
// short-circuits once all candidates are found, so reaping a couple of refs no
// longer reads every installed unit's image when config already covers them.
func realReferencedImages(candidates map[string]bool) map[string]bool {
	found := map[string]bool{}
	remaining := len(candidates)
	mark := func(ref string) {
		c := canonRef(ref)
		if candidates[c] && !found[c] {
			found[c] = true
			remaining--
		}
	}
	if cfg, err := config.LoadGlobal(); err == nil {
		for _, s := range cfg.Services {
			mark(s.Image)
			mark(s.PreviousImage)
		}
	}
	if remaining > 0 {
		if customs, err := config.ListCustomServices(); err == nil {
			for _, s := range customs {
				mark(s.Image)
				mark(s.PreviousImage)
				if remaining == 0 {
					break
				}
			}
		}
	}
	if remaining > 0 {
		for _, img := range installedServiceImages() {
			mark(img)
			if remaining == 0 {
				break
			}
		}
	}
	return found
}

// deepTargets reclaims images nothing references any more. The managed tier
// stays inside lerd's catalog: an image whose every tag is an unprotected
// catalog ref lerd recorded pulling, which catches an old mysql:5.7 left behind
// after an upgrade while the protected set keeps the live image and the
// one-back rollback target.
//
// The deep tier drops both gates and reaps every unused image on the host. Most
// of what accumulates is not a catalog image at all: the base layer of a custom
// container (FROM golang:1.25) survives every rebuild of the site that pulled
// it and is stranded the moment the Containerfile changes or the site goes
// away, and lerd's pull ledger never recorded it, so no narrower rule can ever
// see it. Anything a container holds, or that lerd's own config, quadlets or
// tool set names, is protected in both tiers.
func deepTargets(imgs []image, repos, protected, pulled map[string]bool, scope Scope) []Target {
	var out []Target
	for _, img := range imgs {
		refs := removableRefs(img, repos, protected, pulled, scope)
		// Remove every owned tag so the image actually frees on the last one;
		// credit the reclaimable bytes once (on that last removal), since
		// untagging the earlier aliases frees nothing on its own.
		for i, ref := range refs {
			bytes := int64(0)
			if i == len(refs)-1 {
				bytes = reclaimable(img)
			}
			out = append(out, Target{Kind: "image", ID: ref, Desc: describeUnused(img, repos), Owner: ownerOf(img, repos, protected), Bytes: bytes})
		}
	}
	return out
}

// removableRefs returns the tags to remove for an unused image, or nil to keep
// it. A protected tag (a live service image, a rollback target, an installed
// quadlet's image, one of lerd's tool images) always keeps the whole image,
// since an image frees only when its last tag goes. Below ScopeDeep the image
// must additionally be a catalog ref lerd's ledger records pulling, which keeps
// the unattended tiers off anything the user owns.
func removableRefs(img image, repos, protected, pulled map[string]bool, scope Scope) []string {
	// An image a container still holds can't be removed by podman, so keep it even
	// when no service config references it (a container running a catalog image the
	// config no longer names would otherwise be listed forever).
	if len(img.Names) == 0 || inUse(img) {
		return nil
	}
	catalogOnly := scope < ScopeDeep
	refs := make([]string, 0, len(img.Names))
	lerdPulled := false
	for _, n := range img.Names {
		if protected[canonRef(n)] {
			return nil
		}
		if catalogOnly && !repos[canonRepo(n)] {
			return nil
		}
		if pulled[canonRef(n)] {
			lerdPulled = true
		}
		refs = append(refs, n)
	}
	if catalogOnly && !lerdPulled {
		return nil
	}
	return refs
}

// describeUnused names an unused image for the plan the user confirms, so a
// service upgrade leftover reads differently from the stranded base layer of a
// custom container.
func describeUnused(img image, repos map[string]bool) string {
	for _, n := range img.Names {
		if repos[canonRepo(n)] {
			return "unused service image"
		}
	}
	return "unused image"
}

// lerdOwned reports whether an image belongs to the stack lerd manages: one it
// built, a PHP base it pulled, an image one of its quadlets or tools names, or
// a service image from the catalog. Everything else on the host is someone
// else's, including the build bases a custom container pulled, which lerd never
// chose and cannot re-pull on its own.
func lerdOwned(img image, repos, protected map[string]bool) bool {
	if isLerd(img) || baseName(img) != "" {
		return true
	}
	for _, n := range img.Names {
		if protected[canonRef(n)] || repos[canonRepo(n)] {
			return true
		}
	}
	return false
}

// usedImage describes one of lerd's own images for the usage breakdown, or
// reports false for an image that is not lerd's. A dangling image is named by
// its short ID, the only handle it has left.
func usedImage(img image, repos, protected map[string]bool) (UsedImage, bool) {
	if !lerdOwned(img, repos, protected) {
		return UsedImage{}, false
	}
	ref := shortID(img.ID)
	if len(img.Names) > 0 {
		ref = img.Names[0]
	}
	return UsedImage{Ref: ref, InUse: inUse(img), Bytes: reclaimable(img)}, true
}

// ownerOf maps an image to the owner its target is credited to.
func ownerOf(img image, repos, protected map[string]bool) string {
	if lerdOwned(img, repos, protected) {
		return OwnerLerd
	}
	return OwnerOther
}

// canonRef / canonRepo canonicalise an image reference through the shared
// registry parser, so a fully-qualified ref and its short form (mysql:8.4 ==
// docker.io/library/mysql:8.4) compare equal on both sides of every lookup.
func canonRef(ref string) string {
	r, err := registry.ParseImage(ref)
	if err != nil {
		return ref
	}
	return r.Registry + "/" + r.Repo + ":" + r.Tag
}

func canonRepo(ref string) string {
	r, err := registry.ParseImage(ref)
	if err != nil {
		return ref
	}
	return r.Registry + "/" + r.Repo
}

func realServiceRepos() (map[string]bool, error) {
	presets, err := config.ListPresets()
	if err != nil {
		return nil, err
	}
	repos := map[string]bool{}
	add := func(image string) {
		if image != "" {
			repos[canonRepo(image)] = true
		}
	}
	for _, p := range presets {
		add(p.Image)
		for _, v := range p.Versions {
			add(v.Image)
		}
	}
	return repos, nil
}

func realProtectedImages() (map[string]bool, error) {
	prot := map[string]bool{}
	add := func(ref string) {
		if ref != "" {
			prot[canonRef(ref)] = true
		}
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return nil, err
	}
	for _, s := range cfg.Services {
		add(s.Image)
		add(s.PreviousImage)
	}
	customs, err := config.ListCustomServices()
	if err != nil {
		return nil, err
	}
	for _, s := range customs {
		add(s.Image)
		add(s.PreviousImage)
	}
	for _, ref := range installedServiceImages() {
		add(ref)
	}
	for _, ref := range podman.ToolImages() {
		add(ref)
	}
	for _, ref := range containerImages() {
		add(ref)
	}
	return prot, nil
}

// containerImages is the seam tests override; it lists the image of every
// lerd-named container on the host.
var containerImages = podmanContainerImages

// podmanContainerImages returns the image of every lerd-* container, running or
// stopped. Not every image lerd runs comes from a quadlet: a site's workers run
// as plain containers, so without this a stripe-listen sidecar's image reads as
// someone else's on the dashboard.
func podmanContainerImages() []string {
	out, err := podman.Run("ps", "-a", "--filter", "name=lerd-", "--format", "{{.Image}}")
	if err != nil {
		return nil
	}
	var refs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if ref := strings.TrimSpace(line); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

// installedServiceImages returns the current Image= of every installed lerd
// quadlet, so a service that is installed but not running is still protected.
// PHP-FPM units are included: the deep tier reaps any unused image, and a
// stopped site's FPM image would otherwise read as one and cost a full rebuild.
func realInstalledServiceImages() []string {
	entries, err := os.ReadDir(config.QuadletDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "lerd-") || !strings.HasSuffix(name, ".container") {
			continue
		}
		unit := strings.TrimSuffix(name, ".container")
		if img := podman.InstalledImage(unit); img != "" {
			out = append(out, img)
		}
	}
	return out
}
