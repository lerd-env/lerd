package update_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	lerdUpdate "github.com/geodro/lerd/internal/update"
)

// serveReleases points both fetchers at test servers: the stable redirect and
// the API listing GitHub answers for the prerelease lookup. optedIn pins the
// beta checkbox, so no case reads the real config off the machine.
func serveReleases(t *testing.T, stable string, releases []lerdUpdate.GithubReleaseForTest, optedIn bool) {
	t.Helper()

	stableSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String()+"/tag/"+stable, http.StatusFound)
	}))
	t.Cleanup(stableSrv.Close)

	body, _ := json.Marshal(releases)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) //nolint:errcheck
	}))
	t.Cleanup(apiSrv.Close)

	origBeta := lerdUpdate.BetaChannel
	lerdUpdate.BetaChannel = func() bool { return optedIn }
	t.Cleanup(func() { lerdUpdate.BetaChannel = origBeta })

	origRelease, origAPI := lerdUpdate.ReleaseBaseURLs, lerdUpdate.APIBaseURLs
	lerdUpdate.ReleaseBaseURLs = func() []string { return []string{stableSrv.URL} }
	lerdUpdate.APIBaseURLs = func() []string { return []string{apiSrv.URL} }
	t.Cleanup(func() {
		lerdUpdate.ReleaseBaseURLs, lerdUpdate.APIBaseURLs = origRelease, origAPI
	})
}

// Someone who took a beta stays on the beta line without having to remember
// --beta again, and comes off it when the stable release of that cycle lands.
func TestLatestFor(t *testing.T) {
	betaThenStable := []lerdUpdate.GithubReleaseForTest{
		{TagName: "v1.35.0-beta.3", Prerelease: true},
		{TagName: "v1.34.0", Prerelease: false},
	}

	cases := []struct {
		name     string
		current  string
		stable   string
		releases []lerdUpdate.GithubReleaseForTest
		optedIn  bool
		want     string
	}{
		{"beta install follows the next beta", "1.35.0-beta.2", "v1.34.0", betaThenStable, false, "v1.35.0-beta.3"},
		{"stable release ends the beta run", "1.35.0-beta.2", "v1.35.0", betaThenStable, false, "v1.35.0"},
		{"stable install never sees a beta", "1.34.0", "v1.34.0", betaThenStable, false, "v1.34.0"},
		{"dev build of a beta still follows betas", "1.35.0-beta.2-3-gabc123", "v1.34.0", betaThenStable, false, "v1.35.0-beta.3"},
		{"no beta published yet", "1.35.0-beta.2", "v1.34.0", []lerdUpdate.GithubReleaseForTest{{TagName: "v1.34.0"}}, false, "v1.34.0"},
		{"stable install opted into betas", "1.34.0", "v1.34.0", betaThenStable, true, "v1.35.0-beta.3"},
		{"opted in but stable is ahead", "1.34.0", "v1.35.0", betaThenStable, true, "v1.35.0"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			serveReleases(t, c.stable, c.releases, c.optedIn)
			got, err := lerdUpdate.LatestFor(c.current)
			if err != nil {
				t.Fatalf("LatestFor: %v", err)
			}
			if got != c.want {
				t.Errorf("LatestFor(%q) = %q, want %q", c.current, got, c.want)
			}
		})
	}
}
