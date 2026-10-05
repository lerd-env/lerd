package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/atomicfile"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/origin"
	"gopkg.in/yaml.v3"
)

const (
	httpTimeout       = 10 * time.Second
	maxFetchAttempts  = 3
	fetchRetryBackoff = 400 * time.Millisecond
)

// FetchConcurrency is how many store fetches a bulk refresh keeps in flight. The
// whole catalogue is dozens of small files off a static origin, so the cost is
// round trips rather than bandwidth, and a cap this side of a scrape keeps
// raw.githubusercontent.com happy.
const FetchConcurrency = 8

// httpTransport is shared by every fetch. The default keeps only two idle
// connections per host, so a parallel refresh would hand back six of its eight
// connections after each wave and handshake them again on the next.
var httpTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = FetchConcurrency
	return t
}()

// sleepFn is the backoff sleep, a seam so tests don't wait in real time.
var sleepFn = time.Sleep

// Client fetches framework definitions from the remote store. BaseURL is tried
// first; Fallbacks are tried in order if it fails, so a binary can reach the new
// store location after an org move and fall back to the old one before it.
type Client struct {
	BaseURL   string
	Fallbacks []string
}

// Index is the top-level store index listing all available frameworks, and the
// composer packages that ship declarations of their own.
type Index struct {
	Frameworks []IndexEntry               `json:"frameworks"`
	Packages   []config.StorePackageEntry `json:"packages,omitempty"`
}

// IndexEntry describes a single framework available in the store.
type IndexEntry struct {
	Name     string                 `json:"name"`
	Label    string                 `json:"label"`
	Versions []string               `json:"versions"`
	Latest   string                 `json:"latest"`
	Detect   []config.FrameworkRule `json:"detect"`
}

func init() {
	config.RegisterFrameworkFetchHook(autoFetchFramework)
}

// autoFetchFramework downloads a framework definition from the store and saves
// it to the local store directory. Called automatically by config.GetFrameworkForDir
// when a version-specific definition is missing locally.
func autoFetchFramework(name, version string) (*config.Framework, error) {
	client := NewClient()
	fw, err := client.FetchFramework(name, version)
	if err != nil {
		return nil, err
	}
	if err := config.SaveStoreFramework(fw); err != nil {
		return nil, err
	}
	return fw, nil
}

// fetchFrameworkIcon caches a framework's mark, <name>.svg beside the versioned
// definitions it belongs to. The mark is per family rather than per version, so
// it is fetched by name and shared by every version. Best effort throughout: a
// framework that ships no mark, an unreachable store, or markup the sanitizer
// refuses all leave the framework rendering as its label alone.
func (c *Client) fetchFrameworkIcon(name string) {
	if _, ok := config.FrameworkIcon(name); ok {
		return
	}
	data, err := c.fetch(name + ".svg")
	if err != nil {
		return
	}
	_ = config.SaveStoreFrameworkIcon(name, data)
}

// fetchWorkerIcons caches the marks a definition's workers name, under
// workers/<icon>.svg. A worker's icon is often one of the built-in glyphs and
// not a mark at all, so a miss here is the normal case and costs one 404 per
// icon per install. Best effort throughout, exactly like a framework's own mark.
func (c *Client) fetchWorkerIcons(workers map[string]config.FrameworkWorker) {
	for _, w := range workers {
		if w.Icon == "" {
			continue
		}
		if _, ok := config.WorkerIcon(w.Icon); ok {
			continue
		}
		data, err := c.fetch("workers/" + w.Icon + ".svg")
		if err != nil {
			continue
		}
		_ = config.SaveStoreWorkerIcon(w.Icon, data)
	}
}

// NewClient returns a store client with default settings.
func NewClient() *Client {
	urls := origin.StoreBaseURLs()
	return &Client{
		BaseURL:   urls[0],
		Fallbacks: urls[1:],
	}
}

// FetchIndex downloads the store index.
func (c *Client) FetchIndex() (*Index, error) {
	idx, _, err := c.fetchIndex()
	return idx, err
}

// fetchIndex downloads and parses the store index, returning the raw bytes too so
// callers can persist them to the on-disk cache verbatim.
func (c *Client) fetchIndex() (*Index, []byte, error) {
	data, err := c.fetch("index.json")
	if err != nil {
		return nil, nil, fmt.Errorf("fetching store index: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, nil, fmt.Errorf("parsing store index: %w", err)
	}

	return &idx, data, nil
}

// RefreshIndex downloads the store index, updates the local cache, and returns
// it, so offline detection and listing can read the full catalogue without a
// network round trip.
func (c *Client) RefreshIndex() (*Index, error) {
	idx, data, err := c.fetchIndex()
	if err != nil {
		return nil, err
	}
	writeCachedIndex(data)
	return idx, nil
}

// WatchIndex refreshes the cached store index once at startup and then on every
// interval tick. Meant to run as a goroutine from the long-running watcher.
func WatchIndex(interval time.Duration) {
	_, _ = NewClient().RefreshIndex()
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		_, _ = NewClient().RefreshIndex()
	}
}

// CachedIndex reads the locally cached store index without touching the network,
// for callers that need the published catalogue and have not just fetched it.
func CachedIndex() (*Index, error) {
	return loadCachedIndex()
}

// loadCachedIndex reads and parses the locally cached store index.
func loadCachedIndex() (*Index, error) {
	data, err := os.ReadFile(config.StoreIndexFile())
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// writeCachedIndex persists the raw index bytes to the local cache through a
// uniquely named temp and a rename, so an offline reader in another process, or
// a crash mid-write, never sees a truncated file. The unique name matters: a
// fixed one lets two concurrent fetches truncate and fill the same temp, and the
// rename then publishes their blend. Best effort: a cache we cannot write just
// means the next read falls back to the network.
// An unchanged index is still touched, because its mtime is what tells the
// readers whether the cache still speaks for the store: skipping the touch ages
// a catalogue that is refreshing perfectly well past the point where a version
// missing from it counts as evidence, and every resolver starts asking for
// versions the store does not publish again.
func writeCachedIndex(data []byte) {
	path := config.StoreIndexFile()
	wrote, err := atomicfile.WriteIfChanged(path, data, 0o644)
	if err != nil || wrote {
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// FetchFramework downloads a framework definition from the store.
// Always fetches from remote to ensure definitions are up to date.
func (c *Client) FetchFramework(name, version string) (*config.Framework, error) {
	if version == "" {
		// Resolve latest from the index, and keep the copy this pulls: the fetch
		// that resolves a latest version is the one a machine with no cached index
		// makes, and dropping it leaves offline detection blind until the watcher's
		// first refresh hours later.
		idx, err := c.RefreshIndex()
		if err != nil {
			return nil, err
		}
		entry, ok := c.findEntry(idx, name)
		if !ok {
			return nil, fmt.Errorf("framework %q not found in store", name)
		}
		version = entry.Latest
	}

	remotePath := name + "/" + version + ".yaml"
	data, err := c.fetch(remotePath)
	if err != nil {
		return nil, fmt.Errorf("fetching %s@%s: %w", name, version, err)
	}

	var fw config.Framework
	if err := yaml.Unmarshal(data, &fw); err != nil {
		return nil, fmt.Errorf("parsing %s@%s: %w", name, version, err)
	}
	if fw.Name == "" {
		return nil, fmt.Errorf("invalid framework definition for %s@%s: missing name", name, version)
	}
	// Every path that caches a remote definition comes through here, so the mark
	// is pulled alongside it in one place rather than at each of the callers.
	c.fetchFrameworkIcon(name)
	c.fetchWorkerIcons(fw.Workers)

	return &fw, nil
}

// Search filters the store index by a case-insensitive substring match on name or label.
func (c *Client) Search(query string) ([]IndexEntry, error) {
	idx, err := c.FetchIndex()
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(query)
	var results []IndexEntry
	for _, entry := range idx.Frameworks {
		if strings.Contains(strings.ToLower(entry.Name), q) ||
			strings.Contains(strings.ToLower(entry.Label), q) {
			results = append(results, entry)
		}
	}
	return results, nil
}

// DetectFromStore checks the store index for a framework matching the given
// project directory. Returns the matching entry, the resolved version, and true
// if found. The version is auto-detected from composer.lock when possible.
func (c *Client) DetectFromStore(dir string) (*IndexEntry, string, bool) {
	idx, err := c.FetchIndex()
	if err != nil {
		return nil, "", false
	}

	for i, entry := range idx.Frameworks {
		for _, rule := range entry.Detect {
			if config.MatchesDetectRule(dir, rule) {
				version := c.resolveVersion(dir, &entry)
				return &idx.Frameworks[i], version, true
			}
		}
	}
	return nil, "", false
}

// ResolveVersion detects the framework version from detect rules, checking
// composer.json constraints and version_file regex matches. Returns the first
// version that matches one of the available versions, or fallback if none match.
func ResolveVersion(dir string, rules []config.FrameworkRule, available []string, fallback string) string {
	for _, rule := range rules {
		if rule.Composer != "" {
			if ver := DetectFrameworkVersionWithKey(dir, rule.Composer, rule.VersionKey, rule.ComposerSections...); ver != "" {
				for _, v := range available {
					if v == ver {
						return ver
					}
				}
			}
		}
		if rule.VersionFile != "" && rule.VersionPattern != "" {
			if ver := DetectVersionFromFile(dir, rule.VersionFile, rule.VersionPattern); ver != "" {
				for _, v := range available {
					if v == ver {
						return ver
					}
				}
			}
		}
	}
	return fallback
}

func (c *Client) resolveVersion(dir string, entry *IndexEntry) string {
	return ResolveVersion(dir, entry.Detect, entry.Versions, entry.Latest)
}

func (c *Client) findEntry(idx *Index, name string) (*IndexEntry, bool) {
	for i, entry := range idx.Frameworks {
		if entry.Name == name {
			return &idx.Frameworks[i], true
		}
	}
	return nil, false
}

func (c *Client) fetch(path string) ([]byte, error) {
	return c.fetchFrom(append([]string{c.BaseURL}, c.Fallbacks...), path)
}

// fetchFrom tries each base in order and returns the first body one serves, so
// a caller reading a part of the store that lives outside the definitions
// directory passes its own bases rather than a path full of parent segments.
func (c *Client) fetchFrom(bases []string, path string) ([]byte, error) {
	client := &http.Client{Timeout: httpTimeout, Transport: httpTransport}
	var errs []string
	for _, base := range bases {
		body, err := fetchWithRetry(client, base+"/"+path)
		if err == nil {
			return body, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, fmt.Errorf("fetching %s: %s", path, strings.Join(errs, "; "))
}

// fetchWithRetry retries a transient fetch failure, a request timeout, a dropped
// connection, or a 5xx, with a short linear backoff before giving up. A slow
// raw.githubusercontent.com response is common when refreshing many definitions at
// once, and would otherwise fail a store entry on the first stall; a definitive 4xx
// (e.g. a removed definition) is not retried.
func fetchWithRetry(client *http.Client, url string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
		body, err := fetchOne(client, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryableFetchErr(err) || attempt == maxFetchAttempts {
			break
		}
		sleepFn(fetchRetryBackoff * time.Duration(attempt))
	}
	return nil, lastErr
}

// httpStatusError carries a non-200 response code so retry classification can tell
// a retryable 5xx from a definitive 4xx.
type httpStatusError struct {
	code int
	url  string
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d from %s", e.code, e.url) }

// retryableFetchErr reports whether an error is worth retrying: any network or
// timeout error (client.Do failing) is transient, an HTTP 5xx is transient, and a
// 4xx is not.
func retryableFetchErr(err error) bool {
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.code >= 500
	}
	return true
}

func fetchOne(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lerd-cli")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{code: resp.StatusCode, url: url}
	}

	return io.ReadAll(resp.Body)
}
