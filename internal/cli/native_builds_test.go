package cli

import (
	"strings"
	"testing"
)

// Installing or updating on the native runtime has no images to build. What
// has to exist is the host binary for each version, so the same run fetches
// the ones that are missing or behind the published pin, and leaves the rest
// alone rather than re-downloading tens of megabytes on every install.
func TestNativeBuildsToFetch(t *testing.T) {
	state := map[string]nativeBuildState{
		"8.1": {present: true, installed: "8.1.34", pinned: "8.1.34"}, // current
		"8.2": {present: false, installed: "", pinned: "8.2.33"},      // never installed
		"8.3": {present: true, installed: "8.3.32", pinned: "8.3.33"}, // behind
		"8.4": {present: true, installed: "", pinned: "8.4.25"},       // no stamp, leave it
		"8.5": {present: true, installed: "8.5.10", pinned: ""},       // no pin reachable
		"8.6": {present: false, installed: "", pinned: ""},            // nothing published
	}
	got := nativeBuildsToFetch(
		[]string{"8.1", "8.2", "8.3", "8.4", "8.5", "8.6"},
		func(v string) nativeBuildState { return state[v] },
	)
	want := "8.2,8.3"
	if strings.Join(got, ",") != want {
		t.Errorf("fetching %v, want %s", got, want)
	}
}

// A collector fix rebuilds a patch that is already published, so the version
// says nothing about whether a machine holds that build. The publish date does,
// and a build with no date recorded predates the field and is fetched once.
func TestNativeBuildsToFetch_PublishDate(t *testing.T) {
	state := map[string]nativeBuildState{
		"8.1": {present: true, installed: "8.1.34", pinned: "8.1.34",
			installedPublished: "2026-09-21T06:00:00Z", publishedAt: "2026-09-21T06:00:00Z"}, // same build
		"8.2": {present: true, installed: "8.2.33", pinned: "8.2.33",
			installedPublished: "2026-09-21T06:00:00Z", publishedAt: "2026-09-28T12:00:00Z"}, // rebuilt
		"8.3": {present: true, installed: "8.3.33", pinned: "8.3.33",
			installedPublished: "", publishedAt: "2026-09-28T12:00:00Z"}, // predates the field
		"8.4": {present: true, installed: "8.4.25", pinned: "8.4.25",
			installedPublished: "", publishedAt: ""}, // no date either side
		"8.5": {present: true, installed: "", pinned: "8.5.10",
			installedPublished: "", publishedAt: "2026-09-28T12:00:00Z"}, // no stamp, leave it
	}
	got := nativeBuildsToFetch(
		[]string{"8.1", "8.2", "8.3", "8.4", "8.5"},
		func(v string) nativeBuildState { return state[v] },
	)
	want := "8.2,8.3"
	if strings.Join(got, ",") != want {
		t.Errorf("fetching %v, want %s", got, want)
	}
}
