package store

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"gopkg.in/yaml.v3"
)

func init() {
	config.RegisterBrowserPresetFetchHook(func(name string) (*config.BrowserPreset, error) {
		return NewClient().FetchBrowserPreset(name)
	})
}

// browserBases returns the store locations browser capture presets are served
// from, a browser directory beside the framework definitions.
func (c *Client) browserBases() []string {
	out := c.packageBases()
	for i, base := range out {
		out[i] = strings.TrimSuffix(base, "/packages") + "/browser"
	}
	return out
}

// FetchBrowserPreset downloads one browser capture preset and caches it.
func (c *Client) FetchBrowserPreset(name string) (*config.BrowserPreset, error) {
	if config.StoreBrowserPresetFile(name) == "" {
		return nil, fmt.Errorf("invalid preset name %q", name)
	}
	data, err := c.fetchFrom(c.browserBases(), name+".yaml")
	if err != nil {
		return nil, fmt.Errorf("fetching browser preset %s: %w", name, err)
	}
	var p config.BrowserPreset
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing browser preset %s: %w", name, err)
	}
	if p.Name != name {
		return nil, fmt.Errorf("browser preset %s declares itself as %q", name, p.Name)
	}
	if err := config.SaveStoreBrowserPreset(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
