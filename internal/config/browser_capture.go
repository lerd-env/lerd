package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Browser capture values a site can opt into on top of uncaught errors and
// unhandled rejections, which are always reported while capture is on.
var (
	BrowserCaptureConsoleLevels  = []string{"error", "warn"}
	BrowserCaptureNetworkClasses = []string{"4xx", "5xx", "failed"}
)

// BrowserCapture is a site's browser capture settings as written in
// .lerd.yaml or the site registry. A nil Enabled or Console keeps the default.
type BrowserCapture struct {
	Enabled *bool    `yaml:"enabled,omitempty"`
	Console []string `yaml:"console"`
	Network []string `yaml:"network"`
	// Navigation reports each page view, SPA navigations included. Nil keeps
	// the default, which is on.
	Navigation *bool `yaml:"navigation,omitempty"`
	// Resources reports an <img>, <script> or stylesheet that failed to load.
	Resources bool `yaml:"resources,omitempty"`
	// Events are DOM events the page should report, such as a frontend
	// library's own error events.
	Events []BrowserCaptureEvent `yaml:"events,omitempty"`
	// Presets names the store presets added to the site, so removing one later
	// takes out what it added and nothing another preset still needs.
	Presets []string `yaml:"presets,omitempty"`
	// Verbose has the script log each capture to the page's console too.
	Verbose bool `yaml:"verbose,omitempty"`
	// Route is where the site serves the script (Route + ".js") and receives
	// reports, for an app that uses /_lerd itself. Empty keeps the default.
	Route string `yaml:"route,omitempty"`
}

// BrowserCaptureEvent is one DOM event to report. Message is a dot path into
// the event, such as detail.response.status, whose value becomes the message.
// Only names and paths are configurable, never code, since the script runs on
// every page of the site.
type BrowserCaptureEvent struct {
	Event   string `yaml:"event" json:"event"`
	Label   string `yaml:"label,omitempty" json:"label"`
	Message string `yaml:"message,omitempty" json:"message"`
}

var (
	browserCaptureEventRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9:._-]{0,99}$`)
	browserCapturePathRE  = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*)(\.[A-Za-z_$][A-Za-z0-9_$]*)*$`)
)

func (e BrowserCaptureEvent) valid() bool {
	return browserCaptureEventRE.MatchString(e.Event) && len(e.Label) <= 100 &&
		(e.Message == "" || browserCapturePathRE.MatchString(e.Message))
}

// DefaultBrowserCaptureRoute is the route a site uses unless it sets its own.
const DefaultBrowserCaptureRoute = "/_lerd/browser"

var browserCaptureRouteRE = regexp.MustCompile(`^(/[A-Za-z0-9._-]+)+$`)

// BrowserCaptureSettings is the resolved form the capture script and the
// dashboard work with.
type BrowserCaptureSettings struct {
	Enabled    bool                  `json:"enabled"`
	Console    []string              `json:"console"`
	Network    []string              `json:"network"`
	Navigation bool                  `json:"navigation"`
	Resources  bool                  `json:"resources"`
	Events     []BrowserCaptureEvent `json:"events"`
	Presets    []string              `json:"presets"`
	Verbose    bool                  `json:"verbose"`
	Route      string                `json:"route"`
}

// Resolve fills in the defaults: enabled, console errors, no network failures.
func (b *BrowserCapture) Resolve() BrowserCaptureSettings {
	out := BrowserCaptureSettings{Enabled: true, Console: []string{"error"}, Network: []string{}, Navigation: true, Events: []BrowserCaptureEvent{}, Presets: []string{}, Route: DefaultBrowserCaptureRoute}
	if b == nil {
		return out
	}
	if b.Enabled != nil {
		out.Enabled = *b.Enabled
	}
	out.Verbose = b.Verbose
	out.Resources = b.Resources
	if b.Presets != nil {
		out.Presets = slices.Clone(b.Presets)
	}
	for _, e := range b.Events {
		if e.valid() {
			out.Events = append(out.Events, e)
		}
	}
	if b.Navigation != nil {
		out.Navigation = *b.Navigation
	}
	if validBrowserCaptureRoute(b.Route) {
		out.Route = b.Route
	}
	if b.Console != nil {
		out.Console = known(b.Console, BrowserCaptureConsoleLevels)
	}
	if b.Network != nil {
		out.Network = known(b.Network, BrowserCaptureNetworkClasses)
	}
	return out
}

func (b *BrowserCapture) clone() *BrowserCapture {
	cp := *b
	if b.Enabled != nil {
		v := *b.Enabled
		cp.Enabled = &v
	}
	if b.Navigation != nil {
		v := *b.Navigation
		cp.Navigation = &v
	}
	cp.Events = slices.Clone(b.Events)
	cp.Presets = slices.Clone(b.Presets)
	cp.Console = slices.Clone(b.Console)
	cp.Network = slices.Clone(b.Network)
	return &cp
}

func known(values, allowed []string) []string {
	out := []string{}
	for _, v := range values {
		if slices.Contains(allowed, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// validBrowserCaptureRoute accepts an absolute path of plain segments, which is
// safe to splice into an nginx location and a script tag.
func validBrowserCaptureRoute(r string) bool {
	return browserCaptureRouteRE.MatchString(r) && !strings.Contains(r, "..")
}

// Validate refuses a value lerd does not know, naming it.
func (s BrowserCaptureSettings) Validate() error {
	for _, e := range s.Events {
		if !e.valid() {
			return fmt.Errorf("invalid event %q (want a DOM event name and an optional dot path such as detail.response.status)", e.Event)
		}
	}
	if s.Route != "" && !validBrowserCaptureRoute(s.Route) {
		return fmt.Errorf("invalid route %q (want an absolute path such as %s)", s.Route, DefaultBrowserCaptureRoute)
	}
	for _, v := range s.Console {
		if !slices.Contains(BrowserCaptureConsoleLevels, v) {
			return fmt.Errorf("unknown console level %q (want one of %v)", v, BrowserCaptureConsoleLevels)
		}
	}
	for _, v := range s.Network {
		if !slices.Contains(BrowserCaptureNetworkClasses, v) {
			return fmt.Errorf("unknown network class %q (want one of %v)", v, BrowserCaptureNetworkClasses)
		}
	}
	return nil
}

// BrowserCaptureFor returns the site's settings: .lerd.yaml's when it sets
// them, else the registry's, else the defaults.
func BrowserCaptureFor(site Site) BrowserCaptureSettings {
	if proj, err := LoadProjectConfig(site.Path); err == nil && proj.BrowserCapture != nil {
		return proj.BrowserCapture.Resolve()
	}
	return site.BrowserCapture.Resolve()
}

// BrowserCaptureInProjectFile reports whether the site's settings are kept in
// its .lerd.yaml, which is the case whenever the project has one.
func BrowserCaptureInProjectFile(site Site) bool {
	_, err := os.Stat(filepath.Join(site.Path, ".lerd.yaml"))
	return err == nil
}

// SaveBrowserCapture stores the site's settings in .lerd.yaml when the project
// has one, and in the site registry otherwise, so no .lerd.yaml is created.
func SaveBrowserCapture(site Site, s BrowserCaptureSettings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	enabled, navigation := s.Enabled, s.Navigation
	bc := &BrowserCapture{
		Enabled:    &enabled,
		Navigation: &navigation,
		Console:    append([]string{}, s.Console...),
		Network:    append([]string{}, s.Network...),
		Verbose:    s.Verbose,
		Resources:  s.Resources,
		Events:     slices.Clone(s.Events),
		Presets:    slices.Clone(s.Presets),
	}
	if s.Route != DefaultBrowserCaptureRoute {
		bc.Route = s.Route
	}
	if BrowserCaptureInProjectFile(site) {
		proj, err := LoadProjectConfig(site.Path)
		if err != nil {
			return err
		}
		proj.BrowserCapture = bc
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
			reg.Sites[i].BrowserCapture = bc
			return SaveSites(reg)
		}
	}
	return fmt.Errorf("site %q not found", site.Name)
}
