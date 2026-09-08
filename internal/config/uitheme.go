package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var validThemeID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// UITheme is a dashboard colour set. Only name and accent are required; the
// dashboard derives every missing tone from the accent and falls back to the
// built-in surfaces, so the shortest usable file is two lines.
type UITheme struct {
	ID              string `yaml:"-"                            json:"id"`
	Name            string `yaml:"name"                         json:"name"`
	Accent          string `yaml:"accent"                       json:"accent"`
	AccentHover     string `yaml:"accent_hover,omitempty"       json:"accent_hover,omitempty"`
	AccentDark      string `yaml:"accent_dark,omitempty"        json:"accent_dark,omitempty"`
	AccentHoverDark string `yaml:"accent_hover_dark,omitempty"  json:"accent_hover_dark,omitempty"`
	Bg              string `yaml:"bg,omitempty"                 json:"bg,omitempty"`
	Card            string `yaml:"card,omitempty"               json:"card,omitempty"`
	Border          string `yaml:"border,omitempty"             json:"border,omitempty"`
	Muted           string `yaml:"muted,omitempty"              json:"muted,omitempty"`
}

// UIThemeError names a file in the themes directory that could not be used and
// why. A theme with a typo in it is reported rather than dropped: the file is
// hand-written, and silently missing from the picker gives the author nothing
// to correct.
type UIThemeError struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// UIThemes reads every theme in ~/.config/lerd/themes, sorted by id.
func UIThemes() ([]UITheme, []UIThemeError) {
	entries, err := os.ReadDir(ThemesDir())
	if err != nil {
		return nil, nil
	}
	var (
		themes []UITheme
		errs   []UIThemeError
	)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		id := strings.TrimSuffix(name, ".yaml")
		data, err := os.ReadFile(filepath.Join(ThemesDir(), name))
		if err != nil {
			errs = append(errs, UIThemeError{File: name, Error: err.Error()})
			continue
		}
		theme, err := parseUITheme(id, data)
		if err != nil {
			errs = append(errs, UIThemeError{File: name, Error: err.Error()})
			continue
		}
		themes = append(themes, *theme)
	}
	sort.Slice(themes, func(i, j int) bool { return themes[i].ID < themes[j].ID })
	sort.Slice(errs, func(i, j int) bool { return errs[i].File < errs[j].File })
	return themes, errs
}

// SaveUITheme validates YAML handed over by the dashboard and writes it as the
// named theme, replacing any file already there.
func SaveUITheme(id string, content []byte) error {
	path, err := uiThemePath(id)
	if err != nil {
		return err
	}
	if _, err := parseUITheme(id, content); err != nil {
		return err
	}
	guardRealWrite(path)
	if err := os.MkdirAll(ThemesDir(), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// DeleteUITheme removes a theme file. A theme that is not there is not an
// error: the file is the state, and the caller wanted it gone.
func DeleteUITheme(id string) error {
	path, err := uiThemePath(id)
	if err != nil {
		return err
	}
	guardRealWrite(path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// uiThemePath turns an id into its file path, refusing anything SaveUITheme
// could not have written. The id indexes straight into a path and arrives over
// HTTP, so the same gate guards reads, writes and deletes.
func uiThemePath(id string) (string, error) {
	if !validThemeID.MatchString(id) {
		return "", fmt.Errorf("invalid theme name %q: must match [a-z0-9][a-z0-9-]*", id)
	}
	return filepath.Join(ThemesDir(), id+".yaml"), nil
}

func parseUITheme(id string, data []byte) (*UITheme, error) {
	if !validThemeID.MatchString(id) {
		return nil, fmt.Errorf("invalid theme name %q: must match [a-z0-9][a-z0-9-]*", id)
	}
	var t UITheme
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	t.ID = id
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return nil, fmt.Errorf("missing required field \"name\"")
	}
	colours := map[string]*string{
		"accent": &t.Accent, "accent_hover": &t.AccentHover,
		"accent_dark": &t.AccentDark, "accent_hover_dark": &t.AccentHoverDark,
		"bg": &t.Bg, "card": &t.Card, "border": &t.Border, "muted": &t.Muted,
	}
	for label, field := range colours {
		if *field == "" {
			continue
		}
		norm := NormalizeBrandColor(*field)
		if norm == "" {
			return nil, fmt.Errorf("%s: only hex colours are accepted, got %q (write it as #rrggbb)", label, *field)
		}
		*field = norm
	}
	if t.Accent == "" {
		return nil, fmt.Errorf("missing required field \"accent\"")
	}
	return &t, nil
}
