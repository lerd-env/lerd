package update

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

func TestIsPrerelease(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"1.19.1", false},
		{"1.20.0-beta.1", true},
		{"1.20.0-rc.1", true},
		{"1.20.0", false},
		{"2.0.0-alpha", true},
	}
	for _, tt := range tests {
		if got := IsPrerelease(tt.in); got != tt.want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestCachedUpdateCheck_skipsPrereleaseForStableUsers pins the defense:
// when the cached latest tag is a prerelease (e.g. /releases/latest
// momentarily redirected to a beta) and the current version is stable,
// no update notification is emitted.
func TestCachedUpdateCheck_skipsPrereleaseForStableUsers(t *testing.T) {
	withTempCache(t, "v1.20.0-beta.1")
	info, err := CachedUpdateCheck("1.19.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil (no update) for stable on cached prerelease, got %+v", info)
	}
}

// TestCachedUpdateCheck_betaUserSeesNewerBeta confirms beta users still get
// notified about newer prereleases, since the filter only applies when the
// caller is on a stable release.
func TestCachedUpdateCheck_betaUserSeesNewerBeta(t *testing.T) {
	withTempCache(t, "v1.20.0-beta.2")
	info, err := CachedUpdateCheck("1.20.0-beta.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected update info for beta-on-beta upgrade")
	}
	if info.LatestVersion != "v1.20.0-beta.2" {
		t.Errorf("got %q, want v1.20.0-beta.2", info.LatestVersion)
	}
}

// TestCachedUpdateCheck_stableUserSeesNewerStable confirms the happy path
// is untouched: stable→stable upgrades still surface.
func TestCachedUpdateCheck_stableUserSeesNewerStable(t *testing.T) {
	withTempCache(t, "v1.19.2")
	info, err := CachedUpdateCheck("1.19.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected update info for stable→stable upgrade")
	}
	if info.LatestVersion != "v1.19.2" {
		t.Errorf("got %q, want v1.19.2", info.LatestVersion)
	}
}

// TestForceUpdateCheck_bypassesCache pins the fix for the "check for updates"
// button doing nothing: even when a fresh cache says we are current, an explicit
// check must query GitHub live and surface a newer release, then refresh the cache.
func TestForceUpdateCheck_bypassesCache(t *testing.T) {
	// Cache says we are already on the latest (1.19.1), well within the 24h TTL.
	withTempCache(t, "v1.19.1")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "changelog") {
			io.WriteString(w, "## [1.20.0] — 2026-01-01\n- new stuff\n") //nolint:errcheck
			return
		}
		w.Header().Set("Location", "https://example.test/releases/tag/v1.20.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	defer stubURLs(&ReleaseBaseURLs, []string{srv.URL})()
	defer stubTagURLs(&changelogURLs, []string{srv.URL + "/changelog"})()

	info, err := ForceUpdateCheck("1.19.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected a live update result bypassing the cache, got nil")
	}
	if info.LatestVersion != "v1.20.0" {
		t.Errorf("LatestVersion = %q, want v1.20.0 (from the network, not the cache)", info.LatestVersion)
	}

	// The live fetch should have rewritten the cache so later cached reads agree.
	if got := cachedLatest(); got != "v1.20.0" {
		t.Errorf("cache after ForceUpdateCheck = %q, want v1.20.0", got)
	}
}

// stubURLs swaps a package-level URL provider for the duration of a test and
// returns a restore func.
func stubURLs(fn *func() []string, urls []string) func() {
	orig := *fn
	*fn = func() []string { return urls }
	return func() { *fn = orig }
}

// stubTagURLs is the tag-accepting variant of stubURLs.
func stubTagURLs(fn *func(string) []string, urls []string) func() {
	orig := *fn
	*fn = func(string) []string { return urls }
	return func() { *fn = orig }
}

// withTempCache pre-seeds the on-disk update-check cache so CachedUpdateCheck
// returns the given tag without hitting the network. Sets XDG_DATA_HOME (the
// var DataDir() reads) so the writes land in the temp dir and never touch
// the user's real ~/.local/share/lerd/update-check.json.
func withTempCache(t *testing.T, tag string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	cacheDir := filepath.Dir(config.UpdateCheckFile())
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := updateCheckState{LatestVersion: tag, CheckedAt: time.Now()}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(config.UpdateCheckFile(), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSummarizeChangelog verifies the short-list parser: section headers are
// kept, bold headlines are extracted with their issue refs, and prose is dropped.
func TestSummarizeChangelog(t *testing.T) {
	sections := `## [1.33.1] - 2026-08-15

This is a long prose paragraph that nobody wants to read in a terminal. It goes
on for several lines and explains the release theme in detail.

### Added

- **A site wizard in the dashboard** (#1473). The ` + "`+`" + ` in Sites opened one screen that browsed the host directories and streamed a link, and now it is a wizard.
- **Offline docs in the dashboard** (#1456). The documentation button opened lerd.sh in an iframe.

### Fixed

- **The doctor repairs the vhost serving a site** (#1421). A vhost is written when a site is linked and nothing ever looked at it again.
- **A key an env file sets twice is reported** (#1403, #1404). A site showed postgres on every surface while the application ran on SQLite.`

	got := SummarizeChangelog(sections)
	want := `### Added
- A site wizard in the dashboard (#1473)
- Offline docs in the dashboard (#1456)

### Fixed
- The doctor repairs the vhost serving a site (#1421)
- A key an env file sets twice is reported (#1403, #1404)`

	if got != want {
		t.Errorf("SummarizeChangelog:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestSummarizeChangelog_empty returns an empty string when there are no entries.
func TestSummarizeChangelog_empty(t *testing.T) {
	if got := SummarizeChangelog(""); got != "" {
		t.Errorf("SummarizeChangelog(\"\") = %q, want \"\"", got)
	}
}
