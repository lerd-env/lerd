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
	// BgLight, CardLight and BorderLight are the light mode's page, cards and
	// lines, which otherwise stay the standard white and greys.
	BgLight     string `yaml:"bg_light,omitempty"     json:"bg_light,omitempty"`
	CardLight   string `yaml:"card_light,omitempty"   json:"card_light,omitempty"`
	BorderLight string `yaml:"border_light,omitempty" json:"border_light,omitempty"`
	// ChromeLight is the tone the rail and the sidebar take in light mode, where
	// the surfaces above are the dark ones. A theme that declares none leaves
	// them white.
	ChromeLight string `yaml:"chrome_light,omitempty" json:"chrome_light,omitempty"`
	// ChromeActive and ChromeLightActive are what the chrome turns while the
	// window has focus, the way Breeze lifts a focused window's header. Without
	// them the chrome keeps one tone.
	ChromeActive      string `yaml:"chrome_active,omitempty"       json:"chrome_active,omitempty"`
	ChromeLightActive string `yaml:"chrome_light_active,omitempty" json:"chrome_light_active,omitempty"`
	// Source says where the theme came from when it was not a file the user can
	// edit. It is never read off a file, so a theme cannot claim to be one.
	Source string `yaml:"-" json:"source,omitempty"`
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
	t, err := decodeUITheme(data)
	if err != nil {
		return nil, err
	}
	t.ID = id
	if t.Name == "" {
		return nil, fmt.Errorf("missing required field \"name\"")
	}
	if t.Accent == "" {
		return nil, fmt.Errorf("missing required field \"accent\"")
	}
	return t, nil
}

// decodeUITheme reads a theme's YAML and normalises every colour in it, without
// asking for the fields a theme file must carry to stand on its own.
func decodeUITheme(data []byte) (*UITheme, error) {
	var t UITheme
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	t.Name = strings.TrimSpace(t.Name)
	for label, field := range t.colours() {
		if *field == "" {
			continue
		}
		norm := NormalizeBrandColor(*field)
		if norm == "" {
			return nil, fmt.Errorf("%s: only hex colours are accepted, got %q (write it as #rrggbb)", label, *field)
		}
		*field = norm
	}
	return &t, nil
}

// colours names every colour field by the key a theme file writes it under.
func (t *UITheme) colours() map[string]*string {
	return map[string]*string{
		"accent": &t.Accent, "accent_hover": &t.AccentHover,
		"accent_dark": &t.AccentDark, "accent_hover_dark": &t.AccentHoverDark,
		"bg": &t.Bg, "card": &t.Card, "border": &t.Border, "muted": &t.Muted,
		"bg_light": &t.BgLight, "card_light": &t.CardLight, "border_light": &t.BorderLight,
		"chrome_light": &t.ChromeLight, "chrome_active": &t.ChromeActive,
		"chrome_light_active": &t.ChromeLightActive,
	}
}
