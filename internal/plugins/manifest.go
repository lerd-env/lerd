// Package plugins discovers user-installed lerd plugins and runs their
// executables. A plugin is a directory holding plugin.yaml plus whatever files
// it names; every surface (CLI, hooks, MCP, API, UI) is declared there and
// served by running one of those files, so a plugin can be written in anything.
package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"gopkg.in/yaml.v3"
)

// ManifestFile is the file that makes a directory a plugin.
const ManifestFile = "plugin.yaml"

// APIVersion is the manifest version this binary understands. A plugin asking
// for a newer one is refused rather than half-loaded.
const APIVersion = 1

type Manifest struct {
	APIVersion  int    `yaml:"api_version" json:"api_version"`
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Lerd is the oldest lerd release the plugin needs, as a constraint like ">=1.37".
	Lerd         string            `yaml:"lerd,omitempty" json:"lerd,omitempty"`
	Commands     []Command         `yaml:"commands,omitempty" json:"commands,omitempty"`
	SiteCommands []SiteCommand     `yaml:"site_commands,omitempty" json:"site_commands,omitempty"`
	Hooks        map[string]string `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	MCP          *MCP              `yaml:"mcp,omitempty" json:"mcp,omitempty"`
	API          []Endpoint        `yaml:"api,omitempty" json:"api,omitempty"`
	UI           *UI               `yaml:"ui,omitempty" json:"ui,omitempty"`
	// Provides maps each job the plugin takes over from lerd to the executable
	// lerd runs for it.
	Provides map[string]string `yaml:"provides,omitempty" json:"provides,omitempty"`
	// Requires names the commands the plugin's scripts call beyond their own
	// interpreters, which lerd reads from each script's first line.
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
}

// Command is a CLI command, invoked as `lerd <plugin>:<name>`.
type Command struct {
	Name  string `yaml:"name" json:"name"`
	Usage string `yaml:"usage,omitempty" json:"usage,omitempty"`
	Run   string `yaml:"run" json:"run"`
}

// SiteCommand runs against one site, from its Commands menu and the palette.
type SiteCommand struct {
	Name        string `yaml:"name" json:"name"`
	Label       string `yaml:"label" json:"label"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Confirm     bool   `yaml:"confirm,omitempty" json:"confirm,omitempty"`
	Run         string `yaml:"run" json:"run"`
	// Timeout raises SiteCommandLimit for this command, up to an hour.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type MCP struct {
	Skill string    `yaml:"skill,omitempty" json:"skill,omitempty"`
	Tools []MCPTool `yaml:"tools,omitempty" json:"tools,omitempty"`
}

type MCPTool struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	InputSchema map[string]any `yaml:"input_schema,omitempty" json:"input_schema,omitempty"`
	Run         string         `yaml:"run" json:"run"`
	// Timeout raises MCPLimit for this tool, up to ten minutes.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// Endpoint is served at /api/plugins/<plugin>/<path>.
type Endpoint struct {
	Path    string   `yaml:"path" json:"path"`
	Methods []string `yaml:"methods,omitempty" json:"methods,omitempty"`
	Run     string   `yaml:"run" json:"run"`
	// LoopbackOnly closes the endpoint to remote dashboards unless the user
	// turned on remote full access, like lerd's own host-action routes.
	LoopbackOnly bool `yaml:"loopback_only,omitempty" json:"loopback_only,omitempty"`
	// Timeout raises APILimit for this endpoint, up to ten minutes.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type UI struct {
	Page    *UIPage `yaml:"page,omitempty" json:"page,omitempty"`
	SiteTab *UIPage `yaml:"site_tab,omitempty" json:"site_tab,omitempty"`
	// PageScript is injected into the pages of the sites the user switched the
	// plugin on for, and runs in those sites' own origin.
	PageScript string `yaml:"page_script,omitempty" json:"page_script,omitempty"`
}

type UIPage struct {
	Title string `yaml:"title" json:"title"`
	Icon  string `yaml:"icon,omitempty" json:"icon,omitempty"`
	Entry string `yaml:"entry" json:"entry"`
}

// Plugin is a validated manifest and the directory it was read from.
type Plugin struct {
	Manifest
	Dir string `json:"dir"`
}

var (
	nameRe     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	apiPathRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9/_-]*$`)
	apiMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
)

// ReadPlugin reads and validates the plugin in dir. Unknown keys are ignored so
// a manifest written for a newer lerd still loads, but an unknown value in a
// known key (an event, a method) is refused by name.
func ReadPlugin(dir string) (*Plugin, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	p := &Plugin{Manifest: m, Dir: dir}
	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("plugin %s: %w", filepath.Base(dir), err)
	}
	return p, nil
}

func (p *Plugin) validate() error {
	if p.APIVersion == 0 {
		return fmt.Errorf("api_version is required")
	}
	if p.APIVersion > APIVersion {
		return fmt.Errorf("api_version %d needs a newer lerd (this one supports %d)", p.APIVersion, APIVersion)
	}
	if !Compatible(p.Lerd) {
		return fmt.Errorf("needs lerd %s (this is %s)", p.Lerd, runningVersion)
	}
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("name %q must be lowercase letters, digits and dashes", p.Name)
	}
	if p.Name != filepath.Base(p.Dir) {
		return fmt.Errorf("name %q must match its directory %q", p.Name, filepath.Base(p.Dir))
	}

	seen := map[string]bool{}
	for _, c := range p.Commands {
		if err := p.checkNamed("command", c.Name, c.Run, seen); err != nil {
			return err
		}
	}
	seen = map[string]bool{}
	for _, c := range p.SiteCommands {
		if err := p.checkNamed("site command", c.Name, c.Run, seen); err != nil {
			return err
		}
		if err := checkLimit(c.Timeout, maxSiteCommandLimit); err != nil {
			return fmt.Errorf("site command %q: %w", c.Name, err)
		}
	}
	for event, run := range p.Hooks {
		if !IsEvent(event) {
			return fmt.Errorf("hook for unknown event %q", event)
		}
		if err := p.checkExecutable(run); err != nil {
			return err
		}
	}
	if p.MCP != nil {
		if err := p.validateMCP(); err != nil {
			return err
		}
	}
	if err := p.validateAPI(); err != nil {
		return err
	}
	for job, run := range p.Provides {
		if !jobs[job] {
			return fmt.Errorf("provides unknown job %q", job)
		}
		if err := p.checkExecutable(run); err != nil {
			return err
		}
	}
	if err := p.checkRequirements(); err != nil {
		return err
	}
	if err := config.CheckPluginDefinitions(p.Dir); err != nil {
		return err
	}
	if p.UI != nil {
		for _, page := range []*UIPage{p.UI.Page, p.UI.SiteTab} {
			if err := p.validatePage(page); err != nil {
				return err
			}
		}
		if p.UI.PageScript != "" {
			if err := p.checkUIFile(p.UI.PageScript); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Plugin) checkNamed(kind, name, run string, seen map[string]bool) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%s name %q must be lowercase letters, digits and dashes", kind, name)
	}
	if seen[name] {
		return fmt.Errorf("%s %q is declared twice", kind, name)
	}
	seen[name] = true
	return p.checkExecutable(run)
}

func (p *Plugin) validateMCP() error {
	if p.MCP.Skill != "" {
		if _, err := p.resolve(p.MCP.Skill); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, tool := range p.MCP.Tools {
		if err := p.checkNamed("mcp tool", tool.Name, tool.Run, seen); err != nil {
			return err
		}
		if err := checkLimit(tool.Timeout, maxEntryLimit); err != nil {
			return fmt.Errorf("mcp tool %q: %w", tool.Name, err)
		}
	}
	return nil
}

func (p *Plugin) validateAPI() error {
	seen := map[string]bool{}
	for _, e := range p.API {
		if !apiPathRe.MatchString(e.Path) || strings.Contains(e.Path, "..") {
			return fmt.Errorf("api path %q must be lowercase letters, digits, '-', '_' and '/'", e.Path)
		}
		if seen[e.Path] {
			return fmt.Errorf("api path %q is declared twice", e.Path)
		}
		seen[e.Path] = true
		for _, m := range e.Methods {
			if !apiMethods[m] {
				return fmt.Errorf("api path %q has unknown method %q", e.Path, m)
			}
		}
		if err := p.checkExecutable(e.Run); err != nil {
			return err
		}
		if err := checkLimit(e.Timeout, maxEntryLimit); err != nil {
			return fmt.Errorf("api path %q: %w", e.Path, err)
		}
	}
	return nil
}

func (p *Plugin) validatePage(page *UIPage) error {
	if page == nil {
		return nil
	}
	if page.Title == "" {
		return fmt.Errorf("ui page %q needs a title", page.Entry)
	}
	files := []string{page.Entry}
	if page.Icon != "" {
		files = append(files, page.Icon)
	}
	for _, rel := range files {
		if err := p.checkUIFile(rel); err != nil {
			return err
		}
	}
	return nil
}

// checkUIFile requires a file the dashboard serves to exist under ui/, the only
// folder served over HTTP, so the rest of the plugin stays private.
func (p *Plugin) checkUIFile(rel string) error {
	if !strings.HasPrefix(rel, UIDir+"/") {
		return fmt.Errorf("ui file %q must live under %s/", rel, UIDir)
	}
	_, err := p.resolve(rel)
	return err
}

// Surfaces names the parts of lerd the plugin adds to, in a fixed order.
func (p *Plugin) Surfaces() []string {
	var out []string
	add := func(name string, has bool) {
		if has {
			out = append(out, name)
		}
	}
	add("commands", len(p.Commands) > 0)
	add("site_commands", len(p.SiteCommands) > 0)
	add("hooks", len(p.Hooks) > 0)
	add("mcp", p.MCP != nil && (len(p.MCP.Tools) > 0 || p.MCP.Skill != ""))
	add("api", len(p.API) > 0)
	add("page", p.UI != nil && p.UI.Page != nil)
	add("site_tab", p.UI != nil && p.UI.SiteTab != nil)
	add("page_script", p.UI != nil && p.UI.PageScript != "")
	add("provides", len(p.Provides) > 0)
	add("services", p.ships(config.PluginServices))
	add("frameworks", p.ships(config.PluginFrameworks))
	return out
}

// PagePath is where a site's pages load plugin page scripts and reach their
// plugins' endpoints, under the site's own origin.
const PagePath = "/_lerd/plugins/"

// UIDir is the one plugin directory the dashboard serves.
const UIDir = "ui"

// resolve turns a manifest path into an absolute one inside the plugin dir,
// refusing anything that would point outside it or does not exist.
func (p *Plugin) resolve(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative to the plugin directory", rel)
	}
	abs := filepath.Join(p.Dir, rel)
	if !strings.HasPrefix(abs, filepath.Clean(p.Dir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q points outside the plugin directory", rel)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("path %q: %w", rel, err)
	}
	return abs, nil
}

func (p *Plugin) checkExecutable(rel string) error {
	abs, err := p.resolve(rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%q must be an executable file", rel)
	}
	return nil
}
