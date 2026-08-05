package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LerdsteadSite is one declared project in lerdstead.yml. Domains are listed
// without the TLD, matching .lerd.yaml. Secured is a *bool so an absent key
// leaves the site's current HTTPS state alone instead of forcing it off.
type LerdsteadSite struct {
	Path       string   `yaml:"path"`
	Domains    []string `yaml:"domains,omitempty"`
	PHPVersion string   `yaml:"php_version,omitempty"`
	Secured    *bool    `yaml:"secured,omitempty"`
	Services   []string `yaml:"services,omitempty"`
}

// Lerdstead is the machine-level declarative site list `lerd apply` reconciles
// against: which projects this machine serves, plus service presets to ensure
// installed ("name" or "name@version") and directories to park.
type Lerdstead struct {
	Sites    []LerdsteadSite `yaml:"sites"`
	Services []string        `yaml:"services,omitempty"`
	Park     []string        `yaml:"park,omitempty"`
}

// LerdsteadFile returns the default lerdstead.yml path next to config.yaml.
func LerdsteadFile() string {
	return filepath.Join(ConfigDir(), "lerdstead.yml")
}

// LoadLerdstead reads and validates a lerdstead.yml. Decoding is strict so a
// typoed key fails loudly instead of silently not applying. Site and park
// paths come back tilde-expanded and absolute-cleaned.
func LoadLerdstead(path string) (*Lerdstead, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var ls Lerdstead
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&ls); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	seen := make(map[string]bool, len(ls.Sites))
	for i := range ls.Sites {
		s := &ls.Sites[i]
		if s.Path == "" {
			return nil, fmt.Errorf("%s: sites[%d] has no path", path, i)
		}
		s.Path = expandHomePath(s.Path)
		if seen[s.Path] {
			return nil, fmt.Errorf("%s: path %s is declared twice", path, s.Path)
		}
		seen[s.Path] = true
	}
	for i := range ls.Park {
		ls.Park[i] = expandHomePath(ls.Park[i])
	}
	return &ls, nil
}

// expandHomePath resolves a leading ~ or ~/ against the user's home directory
// and cleans the result, so declared paths compare stably against the registry.
func expandHomePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
		}
	}
	return filepath.Clean(p)
}
