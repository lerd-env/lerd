package origin

import (
	"fmt"
	"strings"
	"testing"
)

// Every endpoint serves the lerd-env org directly (the geodro move is complete)
// and never returns an empty list that would panic store.NewClient's urls[0].
func TestAllEndpointsServeLerdEnv(t *testing.T) {
	lists := map[string][]string{
		"framework-store": StoreBaseURLs(),
		"service-store":   ServiceStoreBaseURLs(),
		"releases":        ReleaseBaseURLs(),
		"downloads":       ReleaseDownloadBases(),
		"api":             ReleaseAPIBaseURLs(),
		"changelog":       ChangelogURLs(""),
		"baseimage":       BaseImageRefs("85", "h"),
	}
	for name, got := range lists {
		if len(got) == 0 {
			t.Fatalf("%s: empty base list", name)
		}
		if !strings.Contains(got[0], "lerd-env") {
			t.Errorf("%s: primary %q is not the lerd-env location", name, got[0])
		}
		for _, u := range got {
			if strings.Contains(u, "geodro") {
				t.Errorf("%s: must not reference geodro, got %q", name, u)
			}
		}
	}
}

func TestBaseImageRefFormat(t *testing.T) {
	refs := BaseImageRefs("84", "abc")
	if len(refs) != 1 || refs[0] != "ghcr.io/lerd-env/lerd-php84-fpm-base:abc" {
		t.Errorf("base ref = %v, want [ghcr.io/lerd-env/lerd-php84-fpm-base:abc]", refs)
	}
}

func TestBaseImageRegistryOverride(t *testing.T) {
	t.Setenv("LERD_BASE_IMAGE_REGISTRY", "registry.example/mirror")
	refs := BaseImageRefs("85", "h")
	if len(refs) != 1 || refs[0] != "registry.example/mirror/lerd-php85-fpm-base:h" {
		t.Errorf("override base ref = %v", refs)
	}
}

func TestStoreEnvOverrideReplacesList(t *testing.T) {
	t.Setenv("LERD_STORE_BASE_URL", "https://store.example/a, https://store.example/b")
	got := StoreBaseURLs()
	want := []string{"https://store.example/a", "https://store.example/b"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("override list = %v, want %v", got, want)
	}
}

func TestServiceStoreEnvOverride(t *testing.T) {
	t.Setenv("LERD_SERVICES_BASE_URL", "https://svc.example/a, https://svc.example/b")
	got := ServiceStoreBaseURLs()
	want := []string{"https://svc.example/a", "https://svc.example/b"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("service override list = %v, want %v", got, want)
	}
}

// A malformed override (only commas/whitespace) must be ignored and fall back to
// the default, never an empty list that would panic store.NewClient's urls[0].
func TestEnvOverrideIgnoredWhenEmpty(t *testing.T) {
	t.Setenv("LERD_STORE_BASE_URL", " , , ")
	got := StoreBaseURLs()
	if len(got) == 0 || !strings.Contains(got[0], "lerd-env") {
		t.Fatalf("empty override must fall back to the lerd-env default, got %v", got)
	}
}

// ChangelogURLs pins to docs/changelog.md at the given tag, not the root
// symlink at main, since raw.githubusercontent does not follow symlinks.
func TestChangelogURLsAtTag(t *testing.T) {
	got := ChangelogURLs("1.33.1")
	if len(got) != 1 {
		t.Fatalf("got %d URLs, want 1", len(got))
	}
	want := "https://raw.githubusercontent.com/lerd-env/lerd/v1.33.1/docs/changelog.md"
	if got[0] != want {
		t.Errorf("got %q, want %q", got[0], want)
	}
}

func TestChangelogURLsEmptyTagFallsBackToMain(t *testing.T) {
	got := ChangelogURLs("")
	if len(got) != 1 {
		t.Fatalf("got %d URLs, want 1", len(got))
	}
	want := "https://raw.githubusercontent.com/lerd-env/lerd/main/docs/changelog.md"
	if got[0] != want {
		t.Errorf("got %q, want %q", got[0], want)
	}
}

// The container DNS probe resolves the store's own host, so a self-hosted
// store is probed instead of a name the install never fetches.
func TestStoreHostFollowsTheStoreOverride(t *testing.T) {
	if got := StoreHost(); got != "raw.githubusercontent.com" {
		t.Errorf("default store host = %q, want raw.githubusercontent.com", got)
	}
	t.Setenv("LERD_STORE_BASE_URL", "https://store.example.test:8443/frameworks")
	if got := StoreHost(); got != "store.example.test" {
		t.Errorf("overridden store host = %q, want store.example.test", got)
	}
}

// A store base that is not a URL leaves nothing to probe; the caller skips
// rather than resolving an empty name.
func TestStoreHostEmptyWhenBaseIsNotAURL(t *testing.T) {
	t.Setenv("LERD_STORE_BASE_URL", "not a url")
	if got := StoreHost(); got != "" {
		t.Errorf("store host = %q, want empty", got)
	}
}

// The store publishes a rendered tree per schema. This binary asks for the one
// it understands and falls back to the unprefixed path, which is schema 1 and
// what every binary up to 1.35.0 fetches, so a store that has not published the
// newer schema yet still resolves.
func TestServiceStoreBaseURLs_PrefersItsOwnSchema(t *testing.T) {
	t.Setenv("LERD_SERVICES_BASE_URL", "")
	got := ServiceStoreBaseURLs()
	if len(got) != 2 {
		t.Fatalf("expected the schema path and the legacy fallback, got %v", got)
	}
	want := fmt.Sprintf("/main/schema/%d/services", StoreSchema)
	if !strings.HasSuffix(got[0], want) {
		t.Errorf("first base should be the schema tree %q, got %q", want, got[0])
	}
	if !strings.HasSuffix(got[1], "/main/services") {
		t.Errorf("fallback should be the legacy unprefixed path, got %q", got[1])
	}
}

// An explicit override is taken as given: a mirror or a test server carries
// whatever layout its owner published.
func TestServiceStoreBaseURLs_OverrideWins(t *testing.T) {
	t.Setenv("LERD_SERVICES_BASE_URL", "https://mirror.example/services")
	got := ServiceStoreBaseURLs()
	if len(got) != 1 || got[0] != "https://mirror.example/services" {
		t.Errorf("override should be used verbatim, got %v", got)
	}
}

// The chain has to step through every schema down to the unprefixed path, not
// jump from the newest to the oldest. A definition introduced at schema 2 lives
// only in that tree and is deliberately absent from the legacy one, so a schema 3
// binary that skipped schema 2 would fail to fetch a preset it can run.
func TestSchemaBases_StepsThroughEverySchema(t *testing.T) {
	got := schemaBases("https://store.example", 3, "services")
	want := []string{
		"https://store.example/schema/3/services",
		"https://store.example/schema/2/services",
		"https://store.example/services",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("base %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// Schema 1 is the unprefixed path itself, so it has no prefixed tree.
func TestSchemaBases_SchemaOneIsTheLegacyPathAlone(t *testing.T) {
	got := schemaBases("https://store.example", 1, "services")
	if len(got) != 1 || got[0] != "https://store.example/services" {
		t.Errorf("got %v, want just the unprefixed path", got)
	}
}
