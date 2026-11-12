package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/imagepull"
)

// The forced php:rebuild ignores the offline gate, so an offline install with
// a stale image used to pull every base and rebuild every version.
func TestRebuildStalePHPImagesOfflineLeavesTheImagesAlone(t *testing.T) {
	imagepull.SetOffline(true)
	t.Cleanup(func() { imagepull.SetOffline(false) })

	ran := false
	rebuildStalePHPImages(func() error { ran = true; return nil })

	if ran {
		t.Error("offline install force-rebuilt the PHP images")
	}
}

func TestRebuildStalePHPImagesOnlineRebuilds(t *testing.T) {
	ran := false
	rebuildStalePHPImages(func() error { ran = true; return nil })

	if !ran {
		t.Error("install did not rebuild stale PHP images")
	}
}

// An image already in the store is refreshed, not downloaded, so the
// disclosure must not announce it.
func TestWithoutKeptImagesDropsWhatOfflineKeeps(t *testing.T) {
	plan := imagepull.Plan{
		imagepull.Pull("docker.io/library/nginx:alpine", "web server"),
		imagepull.Pull("docker.io/library/alpine:latest", "dns base"),
	}
	kept := func(ref string) bool { return ref == "docker.io/library/nginx:alpine" }

	got := withoutKeptImages(plan, kept)

	if len(got) != 1 || got[0].Ref != "docker.io/library/alpine:latest" {
		t.Errorf("plan = %+v, want only the alpine pull", got)
	}
}

// Online install still pulls nginx to refresh it, but only announces the pull
// when the image is not in the store yet.
func TestWebServerPullPlanAnnouncesOnlyAMissingImage(t *testing.T) {
	t.Setenv("LERD_OFFLINE", "")
	orig := installImageInStore
	t.Cleanup(func() { installImageInStore = orig })
	const nginx = "docker.io/library/nginx:alpine"

	installImageInStore = func(string) bool { return true }
	if got := webServerPullPlan(nginx); len(got) != 0 {
		t.Errorf("local image: plan = %+v, want nothing announced", got)
	}

	installImageInStore = func(string) bool { return false }
	if got := webServerPullPlan(nginx); len(got) != 1 || got[0].Ref != nginx {
		t.Errorf("missing image: plan = %+v, want the nginx pull", got)
	}
}
