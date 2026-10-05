package config

import (
	"fmt"
	"slices"
)

// The values the debug bar's global settings accept.
var (
	DebugbarStyles  = []string{"dock", "compact"}
	DebugbarEdges   = []string{"bottom", "top"}
	DebugbarCorners = []string{"bottom-right", "bottom-left", "top-right", "top-left"}
	DebugbarThemes  = []string{"auto", "light", "dark"}
)

// Debugbar is how the debug bar looks, as written in config.yaml. An empty
// field keeps the default, the first value its list names.
type Debugbar struct {
	// Style is a floating dock or a compact strip along an edge.
	Style string `yaml:"style,omitempty" mapstructure:"style" json:"style"`
	// Edge is the edge the compact strip sits on.
	Edge string `yaml:"edge,omitempty" mapstructure:"edge" json:"edge"`
	// Corner is where the minimised bar goes until someone drags it elsewhere.
	Corner string `yaml:"corner,omitempty" mapstructure:"corner" json:"corner"`
	Theme  string `yaml:"theme,omitempty" mapstructure:"theme" json:"theme"`
}

// Resolve fills in the defaults and drops a value lerd does not know.
func (d Debugbar) Resolve() Debugbar {
	pick := func(v string, allowed []string) string {
		if slices.Contains(allowed, v) {
			return v
		}
		return allowed[0]
	}
	return Debugbar{
		Style:  pick(d.Style, DebugbarStyles),
		Edge:   pick(d.Edge, DebugbarEdges),
		Corner: pick(d.Corner, DebugbarCorners),
		Theme:  pick(d.Theme, DebugbarThemes),
	}
}

// Validate refuses a value lerd does not know, naming it.
func (d Debugbar) Validate() error {
	for _, f := range []struct {
		name, value string
		allowed     []string
	}{
		{"style", d.Style, DebugbarStyles},
		{"edge", d.Edge, DebugbarEdges},
		{"corner", d.Corner, DebugbarCorners},
		{"theme", d.Theme, DebugbarThemes},
	} {
		if f.value != "" && !slices.Contains(f.allowed, f.value) {
			return fmt.Errorf("unknown debug bar %s %q (want one of %v)", f.name, f.value, f.allowed)
		}
	}
	return nil
}

// DebugbarFor reports whether the site shows the debug bar: .lerd.yaml's
// devtools.debugbar when it sets one, else the registry's.
func DebugbarFor(site Site) bool {
	if proj, err := LoadProjectConfig(site.Path); err == nil && proj.Devtools != nil && proj.Devtools.Debugbar != nil {
		return *proj.Devtools.Debugbar
	}
	return site.Debugbar
}

// SaveDebugbar stores the site's setting in .lerd.yaml when the project has
// one, and in the site registry otherwise, so no .lerd.yaml is created.
func SaveDebugbar(site Site, on bool) error {
	if BrowserCaptureInProjectFile(site) {
		proj, err := LoadProjectConfig(site.Path)
		if err != nil {
			return err
		}
		if proj.Devtools == nil {
			proj.Devtools = &ProjectDevtools{}
		}
		proj.Devtools.Debugbar = &on
		return SaveProjectConfig(site.Path, proj)
	}
	siteWriteMu.Lock()
	defer siteWriteMu.Unlock()
	reg, err := LoadSites()
	if err != nil {
		return err
	}
	for i := range reg.Sites {
		if reg.Sites[i].Name == site.Name {
			reg.Sites[i].Debugbar = on
			return SaveSites(reg)
		}
	}
	return fmt.Errorf("site %q not found", site.Name)
}
