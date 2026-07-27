package podman

import (
	"errors"
	"testing"
)

// stubBaseDigests installs the label reader and the two registry seams
// BaseImageFreshness reads, and reports which refs were asked for.
func stubBaseDigests(t *testing.T, builtLabel string, cached map[string]string, live map[string]string) *[]string {
	t.Helper()
	origLabel, origCached, origFetch := imageLabelFn, cachedManifestDigestFn, refreshManifestDigestFn
	origThrough := manifestDigestFn
	t.Cleanup(func() {
		imageLabelFn, cachedManifestDigestFn, refreshManifestDigestFn = origLabel, origCached, origFetch
		manifestDigestFn = origThrough
	})

	// The label memo outlives a single test, so each stub starts from empty.
	baseLabelMu.Lock()
	baseLabelMemo = map[string]baseLabelEntry{}
	baseLabelMu.Unlock()

	asked := []string{}
	imageLabelFn = func(_, key string) string {
		if key == fpmBaseDigestLabel {
			return builtLabel
		}
		return ""
	}
	cachedManifestDigestFn = func(ref string) (string, bool) {
		d, ok := cached[ref]
		return d, ok
	}
	refreshManifestDigestFn = func(ref string) (string, error) {
		asked = append(asked, ref)
		d, ok := live[ref]
		if !ok {
			return "", errors.New("unreachable")
		}
		return d, nil
	}
	// Read-through: the cache first, the registry behind it.
	manifestDigestFn = func(ref string) (string, error) {
		if d, ok := cached[ref]; ok {
			return d, nil
		}
		return refreshManifestDigestFn(ref)
	}
	return &asked
}

func baseRefFor(t *testing.T, version string) string {
	t.Helper()
	refs := baseImageRefs(version)
	if len(refs) == 0 {
		t.Fatal("no base image refs for the current recipe")
	}
	return refs[0]
}

func TestBaseImageFreshness_StaleWhenDigestMoved(t *testing.T) {
	ref := baseRefFor(t, "8.4")
	stubBaseDigests(t, "sha256:old", map[string]string{ref: "sha256:new"}, nil)

	st := BaseImageFreshness("8.4")
	if st == nil {
		t.Fatal("expected a status when both digests are known")
	}
	if !st.Stale {
		t.Errorf("a moved digest must read as stale: %+v", st)
	}
	if st.BuiltDigest != "sha256:old" || st.LatestDigest != "sha256:new" {
		t.Errorf("unexpected digests: %+v", st)
	}
	if st.Ref != ref {
		t.Errorf("ref = %q, want %q", st.Ref, ref)
	}
}

func TestBaseImageFreshness_FreshWhenDigestMatches(t *testing.T) {
	ref := baseRefFor(t, "8.4")
	stubBaseDigests(t, "sha256:same", map[string]string{ref: "SHA256:SAME"}, nil)

	st := BaseImageFreshness("8.4")
	if st == nil {
		t.Fatal("expected a status")
	}
	// Registry casing differences are not a republished base.
	if st.Stale {
		t.Errorf("matching digests must not read as stale: %+v", st)
	}
}

// An image built locally (or by a lerd old enough to predate the label) carries
// no recorded base, and there is nothing to compare it against. Reporting it as
// stale would offer a rebuild that changes nothing.
func TestBaseImageFreshness_UnlabelledImageHasNoSignal(t *testing.T) {
	ref := baseRefFor(t, "8.4")
	stubBaseDigests(t, "", map[string]string{ref: "sha256:new"}, nil)

	if st := BaseImageFreshness("8.4"); st != nil {
		t.Errorf("an unlabelled image must produce no signal, got %+v", st)
	}
}

// The status path must never wait on the registry: with nothing cached it
// answers nil and leaves the fetch to the background refresh.
func TestBaseImageFreshness_ColdCacheDoesNotBlock(t *testing.T) {
	ref := baseRefFor(t, "8.4")
	stubBaseDigests(t, "sha256:old", nil, map[string]string{ref: "sha256:new"})

	if st := BaseImageFreshness("8.4"); st != nil {
		t.Errorf("a cold cache must answer nil, got %+v", st)
	}
}

// Doctor has no later snapshot to pick up a background refresh, so its check
// waits on the registry when the cache has nothing.
func TestCheckBaseImageFreshness_ReadsThroughOnColdCache(t *testing.T) {
	ref := baseRefFor(t, "8.4")
	asked := stubBaseDigests(t, "sha256:old", nil, map[string]string{ref: "sha256:new"})

	st := CheckBaseImageFreshness("8.4")
	if st == nil || !st.Stale {
		t.Fatalf("expected a stale status, got %+v", st)
	}
	if len(*asked) != 1 {
		t.Errorf("expected one registry read, got %v", *asked)
	}
}

func TestRefreshBaseImageFreshness_ReadsThroughToTheRegistry(t *testing.T) {
	ref := baseRefFor(t, "8.4")
	asked := stubBaseDigests(t, "sha256:old", map[string]string{ref: "sha256:old"}, map[string]string{ref: "sha256:new"})

	st := RefreshBaseImageFreshness("8.4")
	if st == nil {
		t.Fatal("expected a status")
	}
	// The stale cached entry must not be what the manual check answers with.
	if !st.Stale || st.LatestDigest != "sha256:new" {
		t.Errorf("manual check must bypass the cache: %+v", st)
	}
	if len(*asked) != 1 || (*asked)[0] != ref {
		t.Errorf("expected one registry read for %q, got %v", ref, *asked)
	}
}

// An unreachable registry is not evidence of anything; the check goes quiet
// rather than claiming the base moved.
func TestRefreshBaseImageFreshness_UnreachableRegistryIsQuiet(t *testing.T) {
	stubBaseDigests(t, "sha256:old", nil, nil)

	if st := RefreshBaseImageFreshness("8.4"); st != nil {
		t.Errorf("an unreachable registry must produce no signal, got %+v", st)
	}
}

func TestFPMBuildArgs_StampsBaseDigest(t *testing.T) {
	args := fpmBuildArgs("lerd-php84-fpm:local", "abc123", "cust1", "sha256:base", false)
	if !sliceContainsPair(args, "--label", fpmBaseDigestLabel+"=sha256:base") {
		t.Errorf("build args missing the base-digest label\nargs: %v", args)
	}
	// A local build has no base digest, and the label must be emitted empty so a
	// rebuild clears the digest an earlier prebuilt-base build stamped.
	local := fpmBuildArgs("lerd-php84-fpm:local", "abc123", "cust1", "", true)
	if !sliceContainsPair(local, "--label", fpmBaseDigestLabel+"=") {
		t.Errorf("local build must clear the base-digest label\nargs: %v", local)
	}
}
