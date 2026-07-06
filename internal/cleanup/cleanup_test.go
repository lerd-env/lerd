package cleanup

import (
	"errors"
	"testing"
)

// withImages swaps the image-scan and layer-inspect seams for fixtures and
// restores them after. layers maps an image ID to its RootFS layers.
func withImages(t *testing.T, imgs []image, layers map[string][]string) {
	t.Helper()
	scanImages = func() ([]image, error) { return imgs, nil }
	imageLayers = func(ids []string) (map[string][]string, error) {
		m := map[string][]string{}
		for _, id := range ids {
			if l, ok := layers[id]; ok {
				m[id] = l
			}
		}
		return m, nil
	}
	t.Cleanup(func() {
		scanImages = podmanImages
		imageLayers = podmanImageLayers
	})
}

func TestInspect_ReclaimsOnlyOrphanedLerdImages(t *testing.T) {
	withImages(t, []image{
		// orphaned lerd FPM base (tag moved away on rebuild) → reclaim
		{ID: "sha256:aaa", Names: nil, Size: 100, Labels: map[string]string{"dev.lerd.fpm.containerfile-hash": "h1"}},
		// live lerd image (still tagged) → keep
		{ID: "sha256:bbb", Names: []string{"lerd-php84-fpm:local"}, Size: 200, Labels: map[string]string{"dev.lerd.fpm.containerfile-hash": "h2"}},
		// dangling but not lerd → keep (only touch lerd)
		{ID: "sha256:ccc", Names: nil, Size: 400, Labels: nil},
		// tagged non-lerd image → keep
		{ID: "sha256:ddd", Names: []string{"mysql:8.4"}, Size: 800, Labels: nil},
		// orphaned lerd FrankenPHP image; 600 of its 1600 bytes are shared layers,
		// so only the 1000 unique bytes are actually reclaimable → reclaim
		{ID: "sha256:eee", Names: []string{"<none>:<none>"}, Size: 1600, SharedSize: 600, Labels: map[string]string{"dev.lerd.frankenphp.containerfile-hash": "h3"}},
	}, nil)

	p, err := Inspect(false)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, tg := range p.Targets {
		got[tg.ID] = true
	}
	if len(got) != 2 || !got["aaa"] || !got["eee"] {
		t.Fatalf("want exactly orphaned lerd images {aaa,eee}, got %+v", p.Targets)
	}
	// aaa contributes its full 100 (no shared layers); eee contributes 1000
	// (1600 size minus 600 shared) — shared layers are not counted as reclaimed.
	if want := int64(1100); p.ReclaimBytes() != want {
		t.Errorf("ReclaimBytes = %d, want %d", p.ReclaimBytes(), want)
	}
}

// The "only touch lerd" contract: with nothing lerd-built present, the plan is
// empty even when the host is full of other reclaimable podman images.
func TestInspect_NeverTargetsNonLerd(t *testing.T) {
	withImages(t, []image{
		{ID: "sha256:111", Names: nil, Size: 999, Labels: nil},                                                        // dangling non-lerd
		{ID: "sha256:222", Names: []string{"redis:7"}, Size: 999, Labels: nil},                                        // tagged non-lerd
		{ID: "sha256:333", Names: []string{"<none>:<none>"}, Size: 999, Labels: map[string]string{"maintainer": "x"}}, // dangling, foreign label
	}, nil)

	p, err := Inspect(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Targets) != 0 {
		t.Fatalf("non-lerd images must never be targeted, got %+v", p.Targets)
	}
}

// The deep tier reaps every dangling image, lerd's own labelled orphans and
// unlabelled upstream leftovers alike, since a dangling image is unreferenced by
// definition. The safe tier still keeps non-lerd dangling images (proven by
// TestInspect_NeverTargetsNonLerd), so the aggressive reap is opt-out via --safe.
func TestInspect_DeepReapsAllDanglingImages(t *testing.T) {
	withImages(t, []image{
		{ID: "sha256:lerd", Names: nil, Size: 100, Labels: map[string]string{"dev.lerd.fpm.containerfile-hash": "h"}}, // lerd orphan
		{ID: "sha256:mysql", Names: nil, Size: 800}, // old upstream image that lost its tag
		{ID: "sha256:live", Names: []string{"lerd-php84-fpm:local"}, Size: 200, Labels: map[string]string{"dev.lerd.fpm.containerfile-hash": "h2"}},
		{ID: "sha256:tag", Names: []string{"mysql:8.4"}, Size: 900}, // tagged, not dangling → keep
	}, nil)
	serviceRepos = func() (map[string]bool, error) { return map[string]bool{}, nil }
	protectedImages = func() (map[string]bool, error) { return map[string]bool{}, nil }
	t.Cleanup(func() {
		serviceRepos = realServiceRepos
		protectedImages = realProtectedImages
	})

	p, err := Inspect(true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tg := range p.Targets {
		got[tg.ID] = true
	}
	if !got["lerd"] || !got["mysql"] {
		t.Fatalf("deep tier should reap both the lerd orphan and the untagged upstream image, got %+v", p.Targets)
	}
	if got["live"] || got["tag"] {
		t.Errorf("a tagged image must never be reaped, got %+v", p.Targets)
	}
}

// A dangling image a container still holds cannot be removed (podman refuses),
// so it must be kept out of the plan rather than listed forever as "Freed 0 B".
func TestInspect_SkipsInUseDanglingImages(t *testing.T) {
	withImages(t, []image{
		{ID: "sha256:free", Names: nil, Size: 400},                // no container → reap
		{ID: "sha256:held", Names: nil, Size: 500, Containers: 1}, // a container holds it → keep
	}, nil)
	serviceRepos = func() (map[string]bool, error) { return map[string]bool{}, nil }
	protectedImages = func() (map[string]bool, error) { return map[string]bool{}, nil }
	t.Cleanup(func() {
		serviceRepos = realServiceRepos
		protectedImages = realProtectedImages
	})

	p, err := Inspect(true)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tg := range p.Targets {
		got[tg.ID] = true
	}
	if got["held"] {
		t.Error("an image a container holds must be kept (unremovable)")
	}
	if !got["free"] {
		t.Error("a free dangling image should still be reaped")
	}
	// The held dangling image is tallied so the caller can hint a restart frees it.
	if p.Held.Count != 1 || p.Held.Bytes != 500 {
		t.Errorf("Held = %+v, want {Count:1 Bytes:500}", p.Held)
	}
}

// A base image is reaped only when nothing live is built on it — covering both
// an old Containerfile hash for an installed version and a base for a PHP
// version no longer installed. The base whose layers the live image shares is
// kept (untagging it would force a needless re-pull and free nothing).
func TestInspect_ReclaimsOrphanBasesKeepsInUse(t *testing.T) {
	withImages(t,
		[]image{
			// live derived image, built on the current php84 base
			{ID: "local84", Names: []string{"localhost/lerd-php84-fpm:local"}, Size: 700, Labels: map[string]string{"dev.lerd.fpm.containerfile-hash": "h"}},
			// current php84 base: its top layer is in the live image → keep
			{ID: "baseCur", Names: []string{"ghcr.io/geodro/lerd-php84-fpm-base:cur"}, Size: 500},
			// old-hash php84 base: top layer used by nothing live → reclaim
			{ID: "baseOld", Names: []string{"ghcr.io/geodro/lerd-php84-fpm-base:old"}, Size: 500},
			// base for php82, a version no longer installed → reclaim
			{ID: "base82", Names: []string{"ghcr.io/geodro/lerd-php82-fpm-base:cur"}, Size: 500},
		},
		map[string][]string{
			"local84": {"L1", "L2", "L3", "Lcustom"}, // built on current base + custom layer
			"baseCur": {"L1", "L2", "L3"},            // top L3 ∈ live → in use
			"baseOld": {"L1", "L2", "Lold"},          // top Lold ∉ live → orphan
			"base82":  {"L1", "P82a", "P82b"},        // top P82b ∉ live → orphan
		},
	)

	p, err := Inspect(false)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, tg := range p.Targets {
		got[tg.ID] = true
	}
	want := map[string]bool{
		"ghcr.io/geodro/lerd-php84-fpm-base:old": true,
		"ghcr.io/geodro/lerd-php82-fpm-base:cur": true,
	}
	if len(got) != len(want) {
		t.Fatalf("want the two orphan bases reaped, got %+v", p.Targets)
	}
	for ref := range want {
		if !got[ref] {
			t.Errorf("expected %q reaped, missing", ref)
		}
	}
}

func TestApply_RemovesTargetsAndSumsReclaimed(t *testing.T) {
	var removed []string
	removeImage = func(id string) error { removed = append(removed, id); return nil }
	t.Cleanup(func() { removeImage = podmanRemoveImage })

	gotN, got := Apply(Plan{Targets: []Target{{ID: "aaa", Bytes: 100}, {ID: "bbb", Bytes: 250}}})

	if got != 350 {
		t.Errorf("reclaimed = %d, want 350", got)
	}
	if gotN != 2 {
		t.Errorf("removed count = %d, want 2", gotN)
	}
	if len(removed) != 2 || removed[0] != "aaa" || removed[1] != "bbb" {
		t.Errorf("removed = %v, want [aaa bbb]", removed)
	}
}

// A parent image podman refuses while its child is still present is retried on
// a later pass once the child is gone, so a single Apply reclaims the whole
// dangling build chain even when the parent is listed first.
func TestApply_RetriesUntilDependentsFreed(t *testing.T) {
	present := map[string]bool{"child": true, "parent": true}
	removeImage = func(id string) error {
		if id == "parent" && present["child"] {
			return errors.New("image has dependent child images")
		}
		delete(present, id)
		return nil
	}
	t.Cleanup(func() { removeImage = podmanRemoveImage })

	gotN, got := Apply(Plan{Targets: []Target{{ID: "parent", Bytes: 500}, {ID: "child", Bytes: 100}}})

	if got != 600 {
		t.Errorf("reclaimed = %d, want 600 (both freed across passes)", got)
	}
	if gotN != 2 {
		t.Errorf("removed count = %d, want 2", gotN)
	}
	if present["parent"] {
		t.Error("parent should be removed once the child is gone")
	}
}

// A removal that fails (e.g. the image became referenced since Inspect) is
// skipped so one stuck image can't abort the sweep, and its bytes aren't counted.
func TestApply_SkipsFailedRemovalsButContinues(t *testing.T) {
	removeImage = func(id string) error {
		if id == "bad" {
			return errors.New("image is in use")
		}
		return nil
	}
	t.Cleanup(func() { removeImage = podmanRemoveImage })

	gotN, got := Apply(Plan{Targets: []Target{{ID: "bad", Bytes: 100}, {ID: "good", Bytes: 50}}})

	if got != 50 {
		t.Errorf("reclaimed = %d, want 50 (only the successful removal)", got)
	}
	if gotN != 1 {
		t.Errorf("removed count = %d, want 1 (only the successful removal)", gotN)
	}
}
