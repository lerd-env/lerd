package store

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"gopkg.in/yaml.v3"
)

// ResolveStarter finds a listed starter, checks it supports the major, and
// fetches it. A starter the cached index lacks refreshes the index first.
func (c *Client) ResolveStarter(framework, version, name string) (*config.Starter, error) {
	if !config.ValidStarterName(name) {
		return nil, fmt.Errorf("invalid starter name %q", name)
	}
	entry, ok := findStarter(config.StoreStarters(framework), name)
	if !ok {
		if _, err := c.RefreshIndex(); err == nil {
			entry, ok = findStarter(config.StoreStarters(framework), name)
		}
	}
	if !ok {
		return nil, unknownStarterError(framework, name)
	}
	if !entry.Supports(version) {
		return nil, starterRangeError(name, entry.Framework, version)
	}
	s, err := c.FetchStarter(framework, name)
	if err != nil {
		return nil, err
	}
	if !s.Supports(version) {
		return nil, starterRangeError(name, s.Framework, version)
	}
	return s, nil
}

// FetchStarter downloads one starter definition from the store. Starters sit
// beside the framework definitions, under starters/<framework>/<name>.yaml.
func (c *Client) FetchStarter(framework, name string) (*config.Starter, error) {
	if !config.ValidStarterName(name) {
		return nil, fmt.Errorf("invalid starter name %q", name)
	}
	data, err := c.fetchFrom(c.siblingBases("starters"), framework+"/"+name+".yaml")
	if err != nil {
		return nil, fmt.Errorf("fetching starter %s: %w", name, err)
	}
	var s config.Starter
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing starter %s: %w", name, err)
	}
	if s.Starter != name || s.Framework.Name != framework {
		return nil, fmt.Errorf("starter %s/%s declares itself as %s/%s", framework, name, s.Framework.Name, s.Starter)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func findStarter(entries []config.StoreStarterEntry, name string) (config.StoreStarterEntry, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return config.StoreStarterEntry{}, false
}

func unknownStarterError(framework, name string) error {
	var names []string
	for _, e := range config.StoreStarters(framework) {
		names = append(names, e.Name)
	}
	if len(names) == 0 {
		return fmt.Errorf("the store publishes no starters for %s", framework)
	}
	return fmt.Errorf("unknown starter %q for %s, the store publishes: %s", name, framework, strings.Join(names, ", "))
}

func starterRangeError(name string, scope config.FrameworkPackageScope, version string) error {
	var supported string
	switch {
	case scope.Min != "" && scope.Max != "":
		supported = scope.Min + " to " + scope.Max
	case scope.Min != "":
		supported = scope.Min + " and newer"
	case scope.Max != "":
		supported = scope.Max + " and older"
	}
	return fmt.Errorf("starter %s supports %s %s, not %s", name, scope.Name, supported, version)
}
