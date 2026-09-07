package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

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
			return v, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("fetching latest version: %s", strings.Join(errs, "; "))
}

// maxRedirectHops bounds the /releases/latest redirect chain. A repo or org
// rename inserts an extra hop before the tag redirect, so the chain is
// followed to completion instead of reading a single Location (#1296).
const maxRedirectHops = 5

// redirectTransport is the transport the redirect walk uses. Overridable in
// tests so an https hop can be served locally.
var redirectTransport http.RoundTripper

// tagPattern is everything a release tag may contain. The tag ends up in a
// download URL and in an archive filename joined onto a temp dir, so a tag
// carrying a separator, an escape or a shell character is refused outright.
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidTag reports whether tag is safe to interpolate into a download URL and
// an archive path.
func ValidTag(tag string) bool { return tagPattern.MatchString(tag) }

func fetchLatestFrom(base string) (string, error) {
	client := &http.Client{
		Transport: redirectTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	url := base + "/latest"
	secure := strings.HasPrefix(strings.ToLower(url), "https://")
	for hop := 0; hop < maxRedirectHops; hop++ {
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

		if !isRedirect(resp.StatusCode) {
			return "", fmt.Errorf("unexpected status from %s: HTTP %d", url, resp.StatusCode)
		}
		location, err := resp.Location()
		if err != nil {
			return "", fmt.Errorf("no Location header in redirect from %s", url)
		}
		// A chain that has been over https may not drop back to plaintext, and
		// no hop may leave http(s) at all.
		scheme := strings.ToLower(location.Scheme)
		if scheme != "https" && (secure || scheme != "http") {
			return "", fmt.Errorf("refusing %s redirect from %s to %s", scheme, url, location)
		}
		secure = secure || scheme == "https"

		if strings.Contains(location.String(), "/tag/") {
			parts := strings.Split(location.String(), "/tag/")
			if len(parts) != 2 || parts[1] == "" {
				return "", fmt.Errorf("unexpected release URL format: %s", location)
			}
			if !ValidTag(parts[1]) {
				return "", fmt.Errorf("refusing unsafe release tag %q from %s", parts[1], location)
			}
			return parts[1], nil
		}
		url = location.String()
	}
	return "", fmt.Errorf("no release tag after %d redirects from %s/latest", maxRedirectHops, base)
}

func isRedirect(code int) bool {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
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
	token := tokenFor(url)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if err := rateLimitError(url, resp, token != ""); err != nil {
			return "", err
		}
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
		if r.Prerelease && !r.Draft && ValidTag(r.TagName) {
			return r.TagName, nil
		}
	}
	return "", fmt.Errorf("no pre-release found from %s", url)
}

// githubAPIHost is the only host a token may be sent to. A
// LERD_RELEASES_API_URL override points at a mirror or a test rig, and the
// user's credentials have no business going there.
const githubAPIHost = "api.github.com"

// tokenFor returns the GitHub token to authenticate rawURL with, or "" when
// none is set or the URL is not the real GitHub API over https. An
// authenticated call gets 5,000 requests an hour instead of the 60 the whole
// machine shares anonymously.
func tokenFor(rawURL string) string {
	u, err := neturl.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Hostname(), githubAPIHost) {
		return ""
	}
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// rateLimitError turns an exhausted-quota response into an error that says so
// and when the quota comes back, instead of a bare HTTP 403 that reads like a
// broken network. It returns nil for any other failure.
func rateLimitError(url string, resp *http.Response, authenticated bool) error {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		return nil
	}
	msg := fmt.Sprintf("GitHub API rate limit exhausted for %s", url)
	if sec, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		if d := time.Until(time.Unix(sec, 0)); d > 0 {
			msg += fmt.Sprintf(", it resets in %d min", int((d+time.Minute-1)/time.Minute))
		}
	}
	if !authenticated {
		msg += "; set GITHUB_TOKEN to raise the limit"
	}
	return fmt.Errorf("%s", msg)
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
