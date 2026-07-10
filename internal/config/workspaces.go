package config

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Workspaces are a personal, display-only grouping of sites, shown in the web
// UI and the TUI. They are unrelated to site groups (internal/grouping), which
// bind a main site's subdomains and regenerate vhosts and certificates; a
// workspace never touches nginx, domains, certs or .env. Membership is by site
// name, so renaming a domain never orphans it, and a site named in no workspace
// is ungrouped.

var (
	ErrWorkspaceName     = errors.New("workspace name cannot be empty")
	ErrWorkspaceExists   = errors.New("workspace already exists")
	ErrWorkspaceNotFound = errors.New("workspace not found")
	ErrWorkspaceReserved = errors.New(`"none" is reserved for the ungrouped sites`)
)

type Workspace struct {
	Name  string   `yaml:"name"            mapstructure:"name"`
	Sites []string `yaml:"sites,omitempty" mapstructure:"sites"`
}

// globalWriteMu serializes the read-modify-write cycles below within one
// process, so two workspace mutations racing inside lerd-ui cannot clobber each
// other. It is not a cross-process lock: a CLI write landing between this
// process's read and write still wins, as it does for every other config.yaml
// writer.
var globalWriteMu sync.Mutex

// WorkspaceNames returns the workspace names in display order, empty ones included.
func (c *GlobalConfig) WorkspaceNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Workspaces))
	for _, w := range c.Workspaces {
		names = append(names, w.Name)
	}
	return names
}

// WorkspaceOfSite returns the workspace holding the named site, or "" when ungrouped.
func (c *GlobalConfig) WorkspaceOfSite(site string) string {
	if c == nil {
		return ""
	}
	for _, w := range c.Workspaces {
		for _, s := range w.Sites {
			if s == site {
				return w.Name
			}
		}
	}
	return ""
}

// SiteWorkspaceMap maps each grouped site name to its workspace.
func (c *GlobalConfig) SiteWorkspaceMap() map[string]string {
	out := map[string]string{}
	if c == nil {
		return out
	}
	for _, w := range c.Workspaces {
		for _, s := range w.Sites {
			out[s] = w.Name
		}
	}
	return out
}

func (c *GlobalConfig) workspaceIndex(name string) int {
	for i, w := range c.Workspaces {
		if w.Name == name {
			return i
		}
	}
	return -1
}

// cleanWorkspaceName trims and rejects the names a workspace may not take.
// "none" is the CLI's ungroup sentinel and the picker's ungrouped label.
func cleanWorkspaceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrWorkspaceName
	}
	if strings.EqualFold(name, "none") {
		return "", ErrWorkspaceReserved
	}
	return name, nil
}

func (c *GlobalConfig) addWorkspace(name string) error {
	name, err := cleanWorkspaceName(name)
	if err != nil {
		return err
	}
	if c.workspaceIndex(name) >= 0 {
		return fmt.Errorf("%q: %w", name, ErrWorkspaceExists)
	}
	c.Workspaces = append(c.Workspaces, Workspace{Name: name})
	return nil
}

func (c *GlobalConfig) renameWorkspace(old, next string) error {
	next, err := cleanWorkspaceName(next)
	if err != nil {
		return err
	}
	i := c.workspaceIndex(old)
	if i < 0 {
		return fmt.Errorf("%q: %w", old, ErrWorkspaceNotFound)
	}
	if old == next {
		return nil
	}
	if c.workspaceIndex(next) >= 0 {
		return fmt.Errorf("%q: %w", next, ErrWorkspaceExists)
	}
	c.Workspaces[i].Name = next
	return nil
}

// deleteWorkspace drops the workspace. Its members simply become ungrouped; no
// site is ever touched.
func (c *GlobalConfig) deleteWorkspace(name string) {
	i := c.workspaceIndex(name)
	if i < 0 {
		return
	}
	c.Workspaces = append(c.Workspaces[:i], c.Workspaces[i+1:]...)
}

// assignSites moves the named sites into workspace, removing them from whatever
// workspace they were in. An empty workspace ungroups them. A missing target is
// created only when create is set.
func (c *GlobalConfig) assignSites(sites []string, workspace string, create bool) error {
	if workspace != "" {
		var err error
		if workspace, err = cleanWorkspaceName(workspace); err != nil {
			return err
		}
		if c.workspaceIndex(workspace) < 0 {
			if !create {
				return fmt.Errorf("%q: %w", workspace, ErrWorkspaceNotFound)
			}
			if err := c.addWorkspace(workspace); err != nil {
				return err
			}
		}
	}

	moving := make(map[string]bool, len(sites))
	for _, s := range sites {
		moving[s] = true
	}
	for i := range c.Workspaces {
		kept := c.Workspaces[i].Sites[:0]
		for _, s := range c.Workspaces[i].Sites {
			if !moving[s] {
				kept = append(kept, s)
			}
		}
		c.Workspaces[i].Sites = kept
	}
	if workspace == "" {
		return nil
	}

	target := c.workspaceIndex(workspace)
	added := map[string]bool{}
	for _, s := range sites {
		if s == "" || added[s] {
			continue
		}
		added[s] = true
		c.Workspaces[target].Sites = append(c.Workspaces[target].Sites, s)
	}
	return nil
}

// moveWorkspace repositions a workspace in the display order, clamping pos.
func (c *GlobalConfig) moveWorkspace(name string, pos int) error {
	i := c.workspaceIndex(name)
	if i < 0 {
		return fmt.Errorf("%q: %w", name, ErrWorkspaceNotFound)
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(c.Workspaces)-1 {
		pos = len(c.Workspaces) - 1
	}
	w := c.Workspaces[i]
	rest := append(c.Workspaces[:i:i], c.Workspaces[i+1:]...)
	c.Workspaces = append(rest[:pos:pos], append([]Workspace{w}, rest[pos:]...)...)
	return nil
}

// pruneWorkspaceSites drops member names that are no longer in the registry.
// Empty workspaces are kept: the user created them on purpose.
func (c *GlobalConfig) pruneWorkspaceSites(valid map[string]bool) {
	for i := range c.Workspaces {
		kept := c.Workspaces[i].Sites[:0]
		for _, s := range c.Workspaces[i].Sites {
			if valid[s] {
				kept = append(kept, s)
			}
		}
		c.Workspaces[i].Sites = kept
	}
}

// setWorkspaceLayout replaces the whole list, giving both order and membership
// in one write. A site named in two workspaces stays in the first.
func (c *GlobalConfig) setWorkspaceLayout(layout []Workspace) error {
	out := make([]Workspace, 0, len(layout))
	seenName := map[string]bool{}
	seenSite := map[string]bool{}
	for _, w := range layout {
		name, err := cleanWorkspaceName(w.Name)
		if err != nil {
			return err
		}
		if seenName[name] {
			return fmt.Errorf("%q: %w", name, ErrWorkspaceExists)
		}
		seenName[name] = true

		sites := make([]string, 0, len(w.Sites))
		for _, s := range w.Sites {
			if s == "" || seenSite[s] {
				continue
			}
			seenSite[s] = true
			sites = append(sites, s)
		}
		out = append(out, Workspace{Name: name, Sites: sites})
	}
	c.Workspaces = out
	return nil
}

// mutateGlobal runs fn against the on-disk config under the write lock and
// saves the result. Every exported workspace mutator goes through it. fn
// returning errNoWorkspaceChange skips the write and reports success.
func mutateGlobal(fn func(*GlobalConfig) error) error {
	globalWriteMu.Lock()
	defer globalWriteMu.Unlock()
	cfg, err := LoadGlobal()
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		if errors.Is(err, errNoWorkspaceChange) {
			return nil
		}
		return err
	}
	return SaveGlobal(cfg)
}

func AddWorkspace(name string) error {
	return mutateGlobal(func(c *GlobalConfig) error { return c.addWorkspace(name) })
}

func RenameWorkspace(old, next string) error {
	return mutateGlobal(func(c *GlobalConfig) error { return c.renameWorkspace(old, next) })
}

// DeleteWorkspace removes the workspace and ungroups its sites.
func DeleteWorkspace(name string) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		if c.workspaceIndex(name) < 0 {
			return fmt.Errorf("%q: %w", name, ErrWorkspaceNotFound)
		}
		c.deleteWorkspace(name)
		return nil
	})
}

// AssignSiteWorkspace moves the named sites in a single write. An empty
// workspace ungroups them. Only a site's own membership is ever stored; a group
// secondary is never listed, it displays under its main.
func AssignSiteWorkspace(sites []string, workspace string, create bool) error {
	return mutateGlobal(func(c *GlobalConfig) error { return c.assignSites(sites, workspace, create) })
}

func MoveWorkspace(name string, pos int) error {
	return mutateGlobal(func(c *GlobalConfig) error { return c.moveWorkspace(name, pos) })
}

func SetWorkspaceLayout(layout []Workspace) error {
	return mutateGlobal(func(c *GlobalConfig) error { return c.setWorkspaceLayout(layout) })
}

// SetWorkspaceLayoutWith builds the new layout from the workspaces as they are
// on disk, under the write lock. A caller merging a client's layout must use
// this rather than reading the config first: a workspace created between that
// read and the write would otherwise be dropped.
func SetWorkspaceLayoutWith(fn func(current []Workspace) []Workspace) error {
	return mutateGlobal(func(c *GlobalConfig) error { return c.setWorkspaceLayout(fn(c.Workspaces)) })
}

// RemoveSiteFromWorkspaces drops an unlinked site's membership. Without it a
// stale name lingers and a different project relinked under it inherits the
// workspace.
func RemoveSiteFromWorkspaces(site string) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		if c.WorkspaceOfSite(site) == "" {
			return errNoWorkspaceChange
		}
		return c.assignSites([]string{site}, "", false)
	})
}

// errNoWorkspaceChange short-circuits mutateGlobal so a no-op never rewrites
// config.yaml. It never reaches a caller.
var errNoWorkspaceChange = errors.New("no workspace change")

// ListWorkspaces returns the configured workspaces, dropping the names of sites
// that are no longer linked and of group secondaries, which display in their
// main's workspace and hold no membership of their own. The pruning is a
// read-time view; config.yaml is left alone.
func ListWorkspaces() ([]Workspace, error) {
	cfg, err := LoadGlobal()
	if err != nil {
		return nil, err
	}
	reg, err := LoadSites()
	if err != nil {
		return nil, err
	}
	valid := make(map[string]bool, len(reg.Sites))
	for _, s := range reg.Sites {
		valid[s.Name] = s.GroupSubdomain == ""
	}
	cfg.pruneWorkspaceSites(valid)
	return cfg.Workspaces, nil
}
