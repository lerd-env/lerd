package origin

import (
	"strings"
	"testing"
)

// By default (before the org move) every endpoint must serve the old geodro
// location first and the new lerd-env location as a fallback.
func TestDefaultOrderOldFirstNewFallback(t *testing.T) {
	l := New()
	lists := map[string][]string{
		"store":     l.StoreBaseURLs(),
		"releases":  l.ReleaseBaseURLs(),
		"downloads": l.ReleaseDownloadBases(),
		"api":       l.ReleaseAPIBaseURLs(),
		"changelog": l.ChangelogURLs(),
		"baseimage": l.BaseImageRefs("85", "h"),
	}
	for name, got := range lists {
		if len(got) != 2 {
			t.Fatalf("%s: want 2 candidates, got %d (%v)", name, len(got), got)
		}
		if !strings.Contains(got[0], "geodro") {
			t.Errorf("%s: primary %q is not the old geodro location", name, got[0])
		}
		if !strings.Contains(got[1], "lerd-env") {
			t.Errorf("%s: fallback %q is not the new lerd-env location", name, got[1])
		}
	}
}

// After the switch, the new lerd-env location is served first and geodro becomes
// the fallback.
func TestSetPreferNewFlipsOrder(t *testing.T) {
	l := New()
	l.SetPreferNew(true)
	if !l.PreferNew() {
		t.Fatal("PreferNew() should be true after SetPreferNew(true)")
	}
	got := l.StoreBaseURLs()
	if !strings.Contains(got[0], "lerd-env") || !strings.Contains(got[1], "geodro") {
		t.Errorf("after switch want [lerd-env, geodro], got %v", got)
	}
}

func TestDistributionOrgEnvForcesNew(t *testing.T) {
	t.Setenv("LERD_DISTRIBUTION_ORG", "lerd-env")
	l := New()
	if !l.PreferNew() {
		t.Fatal("LERD_DISTRIBUTION_ORG=lerd-env should prefer the new org")
	}
}

// A malformed override (only commas/whitespace) must be ignored and fall back to
// the defaults, never an empty list that would panic store.NewClient's urls[0].
func TestEnvOverrideIgnoredWhenEmpty(t *testing.T) {
	t.Setenv("LERD_STORE_BASE_URL", " , , ")
	got := New().StoreBaseURLs()
	if len(got) < 2 || !strings.Contains(got[0], "geodro") {
		t.Fatalf("empty override must fall back to defaults, got %v", got)
	}
}

func TestStoreEnvOverrideReplacesList(t *testing.T) {
	t.Setenv("LERD_STORE_BASE_URL", "https://store.example/a, https://store.example/b")
	got := New().StoreBaseURLs()
	want := []string{"https://store.example/a", "https://store.example/b"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("override list = %v, want %v", got, want)
	}
}

func TestBaseImageRefFormat(t *testing.T) {
	refs := New().BaseImageRefs("84", "abc")
	if refs[0] != "ghcr.io/geodro/lerd-php84-fpm-base:abc" {
		t.Errorf("primary base ref = %q", refs[0])
	}
	if refs[1] != "ghcr.io/lerd-env/lerd-php84-fpm-base:abc" {
		t.Errorf("fallback base ref = %q", refs[1])
	}
}

// A request that succeeds against a new-org URL flips preference to new-first.
// A request that succeeds against the old org leaves it alone.
func TestNoteFetchedFlipsOnNewOrgSuccess(t *testing.T) {
	l := New()

	// A successful old-org fetch keeps us on old.
	l.NoteFetched(l.StoreBaseURLs()[0]) // geodro (primary while old-first)
	if l.PreferNew() {
		t.Fatal("a successful old-org fetch must not flip to new")
	}

	// A successful new-org fetch (the fallback won) flips to new-first.
	l.NoteFetched(l.StoreBaseURLs()[1]) // lerd-env (fallback while old-first)
	if !l.PreferNew() {
		t.Fatal("a successful new-org fetch must flip to new-first")
	}
	if !strings.Contains(l.StoreBaseURLs()[0], "lerd-env") {
		t.Errorf("after the flip the new org should be served first, got %v", l.StoreBaseURLs())
	}
}

// NoteFetched is one-directional and ignores unrelated or empty bases.
func TestNoteFetchedIgnoresUnrelatedBase(t *testing.T) {
	l := New()
	l.NoteFetched("")
	l.NoteFetched("https://example.com/whatever")
	if l.PreferNew() {
		t.Fatal("an empty or unrelated base must not flip preference")
	}
}

// A new-org win works for a GHCR base-image ref too, not just raw URLs.
func TestNoteFetchedMatchesGHCRRef(t *testing.T) {
	l := New()
	l.NoteFetched(l.BaseImageRefs("85", "h")[1]) // ghcr.io/lerd-env/...
	if !l.PreferNew() {
		t.Fatal("a successful new-org GHCR pull must flip to new-first")
	}
}
