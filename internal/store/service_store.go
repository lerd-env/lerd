package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/origin"
)

// The service-preset store mirrors the framework store but targets the dedicated
// lerd-env/services repo, whose definitions live under a services/ subdir: an
// index.json plus one <name>.yaml per preset (service presets carry their
// versions inline, so unlike frameworks there is no per-version file). It reuses
// the Client fetch/fallback machinery so the two stores can never drift.

// ServiceIndex is the top-level index of the service-preset store.
type ServiceIndex struct {
	Services []ServiceIndexEntry `json:"services"`
}

// ServiceIndexEntry describes one preset available in the store, carrying enough
// to render the install picker (name, description, versions) without fetching
// every preset file. The full definition is fetched on install.
type ServiceIndexEntry struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Family         string                 `json:"family,omitempty"`
	EnvRole        string                 `json:"env_role,omitempty"`
	Dashboard      string                 `json:"dashboard,omitempty"`
	DependsOn      []string               `json:"depends_on,omitempty"`
	Image          string                 `json:"image,omitempty"`
	Versions       []config.PresetVersion `json:"versions,omitempty"`
	DefaultVersion string                 `json:"default_version,omitempty"`
	Category       string                 `json:"category,omitempty"`
	Icon           string                 `json:"icon,omitempty"`
	Color          string                 `json:"color,omitempty"`
	AdminFor       []string               `json:"admin_for,omitempty"`
	AdminRank      int                    `json:"admin_rank,omitempty"`
}

func init() {
	config.RegisterPresetFetchHook(autoFetchPreset)
}

// autoFetchPreset downloads a service preset from the store into the local cache.
// Registered as config's preset-fetch hook so EnsurePreset can pull a store-only
// preset the first time it is installed and refresh a stale cached one.
func autoFetchPreset(name string) error {
	_, err := NewServiceClient().FetchServicePreset(name)
	return err
}

// NewServiceClient returns a store client pointed at the service-preset store.
func NewServiceClient() *Client {
	urls := origin.ServiceStoreBaseURLs()
	return &Client{BaseURL: urls[0], Fallbacks: urls[1:]}
}

// FetchServiceIndex downloads and parses the service-preset store index.
func (c *Client) FetchServiceIndex() (*ServiceIndex, error) {
	data, err := c.fetch("index.json")
	if err != nil {
		return nil, fmt.Errorf("fetching service store index: %w", err)
	}
	var idx ServiceIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing service store index: %w", err)
	}
	return &idx, nil
}

// FetchServicePreset downloads a preset's YAML, validates it against the local
// Preset schema, and saves it verbatim into the store-cache dir. It returns the
// raw bytes on success. Validation happens before the save so a malformed remote
// preset never lands in the cache where the seam would try to serve it.
func (c *Client) FetchServicePreset(name string) ([]byte, error) {
	data, err := c.fetch(name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("fetching service preset %q: %w", name, err)
	}
	if err := config.SaveStorePreset(name, data); err != nil {
		return nil, fmt.Errorf("saving service preset %q: %w", name, err)
	}
	c.fetchServiceIcon(name)
	return data, nil
}

// fetchServiceIcon caches the preset's mark, services/<name>.svg, next to the
// YAML it belongs to. Most presets ship no icon and the ones that do must not
// fail an install over it, so the whole thing is best effort: a missing file, an
// unreachable store, or markup the sanitizer refuses all leave the preset served
// with the built-in glyph its YAML names.
func (c *Client) fetchServiceIcon(name string) {
	data, err := c.fetch(name + ".svg")
	if err != nil {
		return
	}
	_ = config.SaveStorePresetIcon(name, data)
}

// RefreshServiceIcons caches the mark of every preset the store publishes, not
// just the ones installed here, so the discovery grid draws a service's own logo
// before you have ever run it. Only missing marks are fetched, so a repeat sweep
// costs one request per preset that publishes none. Best effort throughout: this
// is decoration, and a store that cannot be reached simply leaves the presets on
// the glyphs their YAML names. Returns how many marks it added.
func (c *Client) RefreshServiceIcons() int {
	idx, err := c.FetchServiceIndex()
	if err != nil {
		return 0
	}
	var added int
	for _, e := range idx.Services {
		if _, ok := config.PresetIcon(e.Name); ok {
			continue
		}
		data, err := c.fetch(e.Name + ".svg")
		if err != nil {
			continue
		}
		if config.SaveStorePresetIcon(e.Name, data) == nil {
			added++
		}
	}
	return added
}

// WatchServiceIcons sweeps the store's marks once at startup and then on every
// interval tick, so a preset published after this binary shipped still shows its
// logo. Meant to run as a goroutine from the long-running watcher.
func WatchServiceIcons(interval time.Duration) {
	NewServiceClient().RefreshServiceIcons()
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		NewServiceClient().RefreshServiceIcons()
	}
}

// SearchServices filters the store index by a case-insensitive substring match
// on name, description, or family.
func (c *Client) SearchServices(query string) ([]ServiceIndexEntry, error) {
	idx, err := c.FetchServiceIndex()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []ServiceIndexEntry
	for _, e := range idx.Services {
		if strings.Contains(strings.ToLower(e.Name), q) ||
			strings.Contains(strings.ToLower(e.Description), q) ||
			strings.Contains(strings.ToLower(e.Family), q) {
			out = append(out, e)
		}
	}
	return out, nil
}
