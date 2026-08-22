package origin

import (
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
