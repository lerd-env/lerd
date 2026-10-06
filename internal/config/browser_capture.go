package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// Browser capture values a site can opt into on top of uncaught errors and
// unhandled rejections, which are always reported while capture is on.
var (
	BrowserCaptureConsoleLevels  = []string{"error", "warn"}
	BrowserCaptureNetworkClasses = []string{"4xx", "5xx", "failed"}
)

// BrowserCapture is a site's browser capture settings as kept in lerd's site
// registry. A nil Enabled or Console keeps the default.
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
	// Presets switches store presets on or off for the site. A preset it does
	// not name is on when the project uses its library, and off otherwise.
	Presets map[string]bool `yaml:"presets,omitempty"`
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

// BrowserCapturePath is where a site's pages load the script (plus ".js") and
// post their reports; nginx hands both to lerd-ui.
const BrowserCapturePath = "/_lerd/browser"

// BrowserCaptureSettings is the resolved form the capture script and the
// dashboard work with.
type BrowserCaptureSettings struct {
	Enabled    bool                  `json:"enabled"`
	Console    []string              `json:"console"`
	Network    []string              `json:"network"`
	Navigation bool                  `json:"navigation"`
	Resources  bool                  `json:"resources"`
	Events     []BrowserCaptureEvent `json:"events"`
	Presets    map[string]bool       `json:"presets"`
}

// Resolve fills in the defaults: off until the site opts in, console errors
// and warnings (Vue and Alpine warn through console.warn), no network failures.
func (b *BrowserCapture) Resolve() BrowserCaptureSettings {
	out := BrowserCaptureSettings{Console: []string{"error", "warn"}, Network: []string{}, Navigation: true, Events: []BrowserCaptureEvent{}, Presets: map[string]bool{}}
	if b == nil {
		return out
	}
	if b.Enabled != nil {
		out.Enabled = *b.Enabled
	}
	out.Resources = b.Resources
	if b.Presets != nil {
		out.Presets = maps.Clone(b.Presets)
	}
	for _, e := range b.Events {
		if e.valid() {
			out.Events = append(out.Events, e)
		}
	}
	if b.Navigation != nil {
		out.Navigation = *b.Navigation
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
	cp.Presets = maps.Clone(b.Presets)
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

// Validate refuses a value lerd does not know, naming it.
func (s BrowserCaptureSettings) Validate() error {
	for _, e := range s.Events {
		if !e.valid() {
			return fmt.Errorf("invalid event %q (want a DOM event name and an optional dot path such as detail.response.status)", e.Event)
		}
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

// BrowserCaptureFor returns the site's settings from lerd's site registry, or
// the defaults. They never live in the project: turning capture on is one
// developer's choice, like the debug switch it works under.
func BrowserCaptureFor(site Site) BrowserCaptureSettings {
	return site.BrowserCapture.Resolve()
}

// SaveBrowserCapture stores the site's settings in lerd's site registry.
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
		Resources:  s.Resources,
		Events:     slices.Clone(s.Events),
		Presets:    maps.Clone(s.Presets),
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
