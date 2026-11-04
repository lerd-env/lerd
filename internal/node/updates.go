package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/atomicfile"
	"github.com/geodro/lerd/internal/config"
)

// releaseIndexURL lists every Node release. All three managers install from
// nodejs.org, so one index answers for each of them. A var so tests can serve it.
var releaseIndexURL = "https://nodejs.org/dist/index.json"

const (
	releaseCacheTTL = 24 * time.Hour
	// releaseRetryAfter holds a failed check back, so an offline host does not
	// wait on the network every time the page opens.
	releaseRetryAfter = time.Hour
	releaseFetchLimit = 8 << 20
)

// VersionStatus is one installed Node major: the newest patch on disk and,
// when known, the newest one published.
type VersionStatus struct {
	Major           string `json:"major"`
	Installed       string `json:"installed"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

// Statuses reports every installed major against the published releases. A
// host whose Node lerd does not manage is never checked, since lerd could not
// act on the answer.
func Statuses(ctx context.Context) []VersionStatus {
	var latest map[string]string
	if Managed() {
		latest = latestByMajor(ctx)
	}
	return versionStatuses(Active().ListFull(), latest)
}

func versionStatuses(full []string, latest map[string]string) []VersionStatus {
	newest := map[string]string{}
	majors := dedupeMajors(full)
	for _, v := range full {
		major := majorOf(v)
		if cur, ok := newest[major]; !ok || versionLess(cur, v) {
			newest[major] = v
		}
	}
	out := make([]VersionStatus, 0, len(majors))
	for _, major := range majors {
		s := VersionStatus{Major: major, Installed: newest[major], Latest: latest[major]}
		s.UpdateAvailable = s.Latest != "" && versionLess(s.Installed, s.Latest)
		out = append(out, s)
	}
	return out
}

func majorOf(version string) string {
	return strings.SplitN(version, ".", 2)[0]
}

// releaseMu serialises refreshes, so two dashboards share one fetch and a slow
// failure cannot write back a cache older than a check that just finished.
var releaseMu sync.Mutex

type releaseCache struct {
	CheckedAt time.Time         `json:"checked_at"`
	Latest    map[string]string `json:"latest"`
}

func releaseCacheFile() string {
	return filepath.Join(config.CacheDir(), "node-releases.json")
}

// latestByMajor is the newest stable release per major, cached for a day. A
// failed fetch keeps the previous answer, which is empty on a host that has
// never had one, so the caller makes no update claim.
func latestByMajor(ctx context.Context) map[string]string {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	var cache releaseCache
	if data, err := os.ReadFile(releaseCacheFile()); err == nil {
		_ = json.Unmarshal(data, &cache)
	}
	if time.Since(cache.CheckedAt) < releaseCacheTTL {
		return cache.Latest
	}
	latest, err := fetchLatestByMajor(ctx)
	if err != nil {
		cache.CheckedAt = time.Now().Add(releaseRetryAfter - releaseCacheTTL)
		writeReleaseCache(cache)
		return cache.Latest
	}
	cache = releaseCache{CheckedAt: time.Now(), Latest: latest}
	writeReleaseCache(cache)
	return latest
}

// CheckReleases re-reads the published releases past the day-long cache. A
// failure leaves the last answer in place and is the caller's to report.
func CheckReleases(ctx context.Context) error {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	latest, err := fetchLatestByMajor(ctx)
	if err != nil {
		return err
	}
	writeReleaseCache(releaseCache{CheckedAt: time.Now(), Latest: latest})
	return nil
}

func writeReleaseCache(cache releaseCache) {
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	path := releaseCacheFile()
	config.GuardRealWrite(path)
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_, _ = atomicfile.WriteIfChanged(path, data, 0o644)
	}
}

func fetchLatestByMajor(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseIndexURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node release index: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, releaseFetchLimit))
	if err != nil {
		return nil, err
	}
	var releases []struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, err
	}
	latest := map[string]string{}
	for _, r := range releases {
		v := strings.TrimPrefix(r.Version, "v")
		// Alpha, beta and nightly builds carry a suffix and sit in the same list.
		if v == "" || strings.Contains(v, "-") {
			continue
		}
		if cur, ok := latest[majorOf(v)]; !ok || versionLess(cur, v) {
			latest[majorOf(v)] = v
		}
	}
	if len(latest) == 0 {
		return nil, errors.New("node release index: no releases")
	}
	return latest, nil
}

// Update installs the newest published patch of an installed major and returns
// the version it ends on. Older patches stay: a site can pin one exactly, and
// removing a version still in use by a running dev server would break it.
func Update(ctx context.Context, major string) (string, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return "", err
	}
	for _, s := range Statuses(ctx) {
		if s.Major == major {
			return applyUpdate(Active(), s, cfg.Node.DefaultVersion)
		}
	}
	return "", fmt.Errorf("Node %s is not installed", major)
}

func applyUpdate(m Manager, s VersionStatus, defaultVersion string) (string, error) {
	if !s.UpdateAvailable {
		return s.Installed, nil
	}
	if err := m.Install(s.Latest); err != nil {
		return "", err
	}
	// fnm's default alias holds one exact version, so it has to be pointed again
	// for a default of "24" to reach the new patch.
	if defaultVersion == s.Major {
		// Nothing reads as outdated now, so a retry would never get back here.
		if err := m.SetDefault(s.Major); err != nil {
			return "", fmt.Errorf("Node %s is installed but the default did not move, run lerd node:use %s: %w", s.Latest, s.Major, err)
		}
	}
	return s.Latest, nil
}
