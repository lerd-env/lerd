package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// httpClient is overridable from tests; production uses a 10s-timeout client.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// cacheTTL is how long ListTags results stay valid on disk before re-fetching.
var cacheTTL = 6 * time.Hour

// dockerHubMaxPages caps pagination for repos with thousands of tags so a
// pathological registry response can't drive unbounded HTTP traffic.
const dockerHubMaxPages = 20

// dockerHubMaxTags caps tags retained in the cache so a malicious registry
// response can't OOM the process.
const dockerHubMaxTags = 5000

// fetchTimeout is the budget for a single registry HTTP call. Token and tag
// list each get their own context so a slow token doesn't starve the list.
const fetchTimeout = 15 * time.Second

func cacheDir() string {
	if d := os.Getenv("LERD_REGISTRY_CACHE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "lerd", "registry-tags")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "lerd", "registry-tags")
}

// AuthRequiredErr is returned when the registry needs credentials we don't
// have. Treated as "no update info" by NewestStable / MaybeNewerTag, but
// distinguishable so future code can surface a "click to authenticate" hint.
type AuthRequiredErr struct{ Registry string }

func (e *AuthRequiredErr) Error() string {
	return "registry " + e.Registry + " needs authentication"
}

// NotFoundErr signals the repo doesn't exist (404). Distinguished from
// transient unreachability so the UI can show a typo hint instead of silently
// hiding the error.
type NotFoundErr struct{ Repo string }

func (e *NotFoundErr) Error() string { return "repository not found: " + e.Repo }

// ListTags fetches available tags for the named image, choosing the backend
// by registry hostname. Errors classify as Auth/NotFound/Unreachable/
// Unsupported so callers can decide whether to surface or swallow.
func ListTags(image string) ([]TagInfo, error) {
	ref, err := ParseImage(image)
	if err != nil {
		return nil, err
	}
	if cached, ok := readCache(ref); ok {
		return cached, nil
	}
	tags, err, _ := listTagsGroup.Do(cacheKey(ref), func() (any, error) {
		var t []TagInfo
		var e error
		switch ref.Registry {
		case "docker.io":
			t, e = listTagsDockerHub(ref)
		case "ghcr.io":
			t, e = listTagsGHCR(ref)
		default:
			return nil, &UnsupportedRegistryErr{Registry: ref.Registry}
		}
		if e != nil {
			return nil, e
		}
		writeCache(ref, t)
		return t, nil
	})
	if err != nil {
		return nil, err
	}
	return tags.([]TagInfo), nil
}

// listTagsGroup deduplicates concurrent ListTags calls for the same image so
// only one HTTP fetch runs at a time.
var listTagsGroup = newSingleflight()

// NewestStable returns the newest version-shaped tag in the registry,
// ignoring update_strategy. Same variant suffix is required. When
// allowMajorUpgrade is false the search stays within the current numeric
// major; when true any newer version qualifies.
func NewestStable(image string, allowMajorUpgrade bool) (*TagInfo, error) {
	ref, err := ParseImage(image)
	if err != nil {
		return nil, err
	}
	tags, err := ListTags(image)
	if err != nil {
		if isQuietRegistryErr(err) {
			return nil, nil
		}
		return nil, err
	}
	current := parseTag(ref.Tag)
	if len(current.Numeric) == 0 {
		return nil, nil
	}
	var best *TagInfo
	var bestNumeric []int
	for i := range tags {
		t := &tags[i]
		ct := parseTag(t.Name)
		if len(ct.Numeric) == 0 {
			continue
		}
		if !allowMajorUpgrade && ct.Numeric[0] != current.Numeric[0] {
			continue
		}
		if !sameVariant(current.Variant, ct.Variant) {
			continue
		}
		if !numericGreater(ct.Numeric, current.Numeric) {
			continue
		}
		if best == nil || numericGreater(ct.Numeric, bestNumeric) {
			best = t
			bestNumeric = ct.Numeric
		}
	}
	return best, nil
}

// MaybeNewerTag is the high-level entry point. Looks at the image's tag,
// queries the registry, applies the strategy, and returns either a newer tag
// to recommend or nil. Quiet errors collapse to (nil, nil).
func MaybeNewerTag(image string, strategy Strategy) (*TagInfo, error) {
	if strategy == StrategyNone || strategy == "" {
		return nil, nil
	}
	ref, err := ParseImage(image)
	if err != nil {
		return nil, err
	}
	tags, err := ListTags(image)
	if err != nil {
		if isQuietRegistryErr(err) {
			return nil, nil
		}
		return nil, err
	}
	current := parseTag(ref.Tag)
	return pickNewer(current, tags, strategy), nil
}

// ---- Docker Hub backend ----------------------------------------------------

type dockerHubResponse struct {
	Next    string             `json:"next"`
	Results []dockerHubTagItem `json:"results"`
}

type dockerHubTagItem struct {
	Name        string                  `json:"name"`
	LastUpdated time.Time               `json:"last_updated"`
	Digest      string                  `json:"digest,omitempty"`
	Images      []dockerHubImageVariant `json:"images,omitempty"`
}

type dockerHubImageVariant struct {
	Digest string `json:"digest"`
}

func listTagsDockerHub(ref ImageRef) ([]TagInfo, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=100&ordering=last_updated", ref.Repo)
	out := make([]TagInfo, 0, 128)
	for page := 0; page < dockerHubMaxPages && url != ""; page++ {
		parsed, err := fetchDockerHubPage(url)
		if err != nil {
			return nil, err
		}
		for _, t := range parsed.Results {
			digest := t.Digest
			if digest == "" && len(t.Images) > 0 {
				digest = t.Images[0].Digest
			}
			out = append(out, TagInfo{Name: t.Name, Pushed: t.LastUpdated, Digest: digest})
			if len(out) >= dockerHubMaxTags {
				return out, nil
			}
		}
		url = parsed.Next
	}
	return out, nil
}

func fetchDockerHubPage(url string) (*dockerHubResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &UnreachableErr{Cause: err}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, &AuthRequiredErr{Registry: "docker.io"}
	case resp.StatusCode == http.StatusNotFound:
		return nil, &NotFoundErr{Repo: strings.TrimPrefix(url, "https://hub.docker.com/v2/repositories/")}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &UnreachableErr{Cause: fmt.Errorf("docker hub rate-limited (429)")}
	case resp.StatusCode >= 500:
		return nil, &UnreachableErr{Cause: fmt.Errorf("docker hub %d", resp.StatusCode)}
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("docker hub %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed dockerHubResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&parsed); err != nil {
		return nil, &UnreachableErr{Cause: err}
	}
	return &parsed, nil
}

// ---- GHCR backend ----------------------------------------------------------

type ghcrTagsResponse struct {
	Tags []string `json:"tags"`
}

type ghcrTokenResponse struct {
	Token string `json:"token"`
}

func listTagsGHCR(ref ImageRef) ([]TagInfo, error) {
	tok, err := fetchGHCRToken(ref.Repo)
	if err != nil {
		return nil, err
	}
	listURL := fmt.Sprintf("https://ghcr.io/v2/%s/tags/list?n=100", ref.Repo)
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &UnreachableErr{Cause: err}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, &AuthRequiredErr{Registry: "ghcr.io"}
	case resp.StatusCode == http.StatusNotFound:
		return nil, &NotFoundErr{Repo: ref.Repo}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &UnreachableErr{Cause: fmt.Errorf("ghcr.io rate-limited (429)")}
	case resp.StatusCode >= 500:
		return nil, &UnreachableErr{Cause: fmt.Errorf("ghcr tags %d", resp.StatusCode)}
	case resp.StatusCode != http.StatusOK:
		return nil, &UnreachableErr{Cause: fmt.Errorf("ghcr tags %d", resp.StatusCode)}
	}
	var parsed ghcrTagsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&parsed); err != nil {
		return nil, &UnreachableErr{Cause: err}
	}
	out := make([]TagInfo, 0, len(parsed.Tags))
	for _, name := range parsed.Tags {
		out = append(out, TagInfo{Name: name})
	}
	return out, nil
}

func fetchGHCRToken(repo string) (string, error) {
	tokenURL := fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull&service=ghcr.io", repo)
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", &UnreachableErr{Cause: err}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return "", &AuthRequiredErr{Registry: "ghcr.io"}
	case resp.StatusCode == http.StatusNotFound:
		return "", &NotFoundErr{Repo: repo}
	case resp.StatusCode != http.StatusOK:
		return "", &UnreachableErr{Cause: fmt.Errorf("ghcr token %d", resp.StatusCode)}
	}
	var tok ghcrTokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok); err != nil {
		return "", &UnreachableErr{Cause: err}
	}
	return tok.Token, nil
}

// ---- Cache -----------------------------------------------------------------

type cachedEntry struct {
	FetchedAt time.Time `json:"fetched_at"`
	Tags      []TagInfo `json:"tags"`
}

// cacheKey hashes registry + repo. Architecture is not included because the
// cached payload is just the tag list; per-arch digests are resolved against
// the local image at the call site, not from cache.
func cacheKey(ref ImageRef) string {
	h := sha256.Sum256([]byte(ref.Registry + "/" + ref.Repo))
	return hex.EncodeToString(h[:])
}

func cachePath(ref ImageRef) string {
	return filepath.Join(cacheDir(), cacheKey(ref)+".json")
}

// cacheMu serialises in-process cache writes so concurrent listTags calls
// can't interleave bytes into the same file. Cross-process safety relies on
// the atomic rename — readers either see the old file or the new one.
var cacheMu sync.Mutex

func readCache(ref ImageRef) ([]TagInfo, bool) {
	data, err := os.ReadFile(cachePath(ref))
	if err != nil {
		return nil, false
	}
	var entry cachedEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if time.Since(entry.FetchedAt) > cacheTTL {
		return nil, false
	}
	return entry.Tags, true
}

func writeCache(ref ImageRef, tags []TagInfo) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		warnCache("mkdir %s: %v", dir, err)
		return
	}
	entry := cachedEntry{FetchedAt: time.Now(), Tags: tags}
	data, err := json.Marshal(entry)
	if err != nil {
		warnCache("marshal: %v", err)
		return
	}
	final := cachePath(ref)
	tmp, err := os.CreateTemp(dir, ".cache-*.tmp")
	if err != nil {
		warnCache("tempfile in %s: %v", dir, err)
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		warnCache("write %s: %v", tmpPath, err)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		warnCache("close %s: %v", tmpPath, err)
		return
	}
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		warnCache("rename %s → %s: %v", tmpPath, final, err)
	}
}

// warnCache rate-limits write-failure logs so a read-only cache dir doesn't
// flood the log on every check. Logs at most once per minute per process.
var (
	cacheWarnMu sync.Mutex
	cacheWarnAt time.Time
)

func warnCache(format string, args ...any) {
	cacheWarnMu.Lock()
	defer cacheWarnMu.Unlock()
	if time.Since(cacheWarnAt) < time.Minute {
		return
	}
	cacheWarnAt = time.Now()
	log.Printf("registry cache write failed: "+format, args...)
}

// ---- singleflight ----------------------------------------------------------

// singleflight is a tiny stand-in for golang.org/x/sync/singleflight to avoid
// adding a dependency. Concurrent Do calls with the same key share one
// execution and result.
type singleflight struct {
	mu sync.Mutex
	m  map[string]*sfCall
}

type sfCall struct {
	wg  sync.WaitGroup
	val any
	err error
}

func newSingleflight() *singleflight {
	return &singleflight{m: map[string]*sfCall{}}
}

func (g *singleflight) Do(key string, fn func() (any, error)) (any, error, bool) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := &sfCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()
	return c.val, c.err, false
}
