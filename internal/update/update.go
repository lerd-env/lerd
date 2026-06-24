package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/origin"
)

// ReleaseBaseURLs returns the GitHub releases bases in priority order, read live
// so a NoteFetched flip reorders them. Overridable in tests.
var ReleaseBaseURLs = origin.ReleaseBaseURLs

// APIBaseURLs returns the GitHub API bases in priority order. Overridable in tests.
var APIBaseURLs = origin.ReleaseAPIBaseURLs

// FetchLatestVersion returns the latest published release tag, trying each
// release base in order until one answers.
func FetchLatestVersion() (string, error) {
	var errs []string
	for _, base := range ReleaseBaseURLs() {
		v, err := fetchLatestFrom(base)
		if err == nil {
			origin.NoteFetched(base)
			return v, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("fetching latest version: %s", strings.Join(errs, "; "))
}

func fetchLatestFrom(base string) (string, error) {
	url := base + "/latest"
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "lerd-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return "", fmt.Errorf("unexpected status from %s: HTTP %d", url, resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no Location header in redirect from %s", url)
	}
	parts := strings.Split(location, "/tag/")
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("unexpected release URL format: %s", location)
	}
	return parts[1], nil
}

// GithubReleaseForTest is exported only so tests in other packages can build
// JSON fixtures. It is not part of the public API.
type GithubReleaseForTest = githubRelease

// githubRelease is a minimal representation of a GitHub release API response.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// FetchLatestPrerelease returns the latest pre-release tag from GitHub.
// Unlike FetchLatestVersion, this calls the GitHub API because the
// /releases/latest redirect only returns stable releases.
func FetchLatestPrerelease() (string, error) {
	var errs []string
	for _, base := range APIBaseURLs() {
		v, err := fetchPrereleaseFrom(base)
		if err == nil {
			origin.NoteFetched(base)
			return v, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("fetching latest pre-release: %s", strings.Join(errs, "; "))
}

func fetchPrereleaseFrom(base string) (string, error) {
	url := base + "/releases"
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "lerd-cli")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status from %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("parsing releases JSON: %w", err)
	}

	for _, r := range releases {
		if r.Prerelease && !r.Draft && r.TagName != "" {
			return r.TagName, nil
		}
	}
	return "", fmt.Errorf("no pre-release found from %s", url)
}

// StripV removes a leading "v" from a version string.
func StripV(v string) string {
	if len(v) > 0 && v[0] == 'v' {
		return v[1:]
	}
	return v
}

// StripGitDescribe removes git-describe suffixes like "-dirty" or "-5-gabcdef"
// while preserving semver pre-release tags like "-beta.1" or "-rc.1".
// Git-describe suffixes contain a commit hash segment starting with "g".
func StripGitDescribe(v string) string {
	for {
		i := strings.LastIndexByte(v, '-')
		if i < 0 {
			break
		}
		suffix := v[i+1:]
		if suffix == "dirty" {
			v = v[:i]
			continue
		}
		// Git describe hash segment: g followed by hex chars.
		// Also strip the preceding commit-count segment (e.g. "-5-gabcdef").
		if len(suffix) > 1 && suffix[0] == 'g' && isHex(suffix[1:]) {
			v = v[:i]
			// Now check if the new last segment is a numeric commit count.
			if j := strings.LastIndexByte(v, '-'); j >= 0 && isNumeric(v[j+1:]) {
				v = v[:j]
			}
			continue
		}
		break
	}
	return v
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
