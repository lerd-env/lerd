package config

import "fmt"

// StreamingHidden returns the names of the sites that must stay off screen
// right now: nothing while streaming mode is off, otherwise PrivateSites.
func (c *GlobalConfig) StreamingHidden(reg *SiteRegistry) map[string]bool {
	if c == nil || !c.UI.StreamingMode {
		return map[string]bool{}
	}
	return c.PrivateSites(reg)
}

// PrivateSites returns the sites streaming mode hides: every private site,
// every member of a private workspace, and the secondaries of a hidden group
// main, which render inside their main.
func (c *GlobalConfig) PrivateSites(reg *SiteRegistry) map[string]bool {
	hidden := map[string]bool{}
	if c == nil || reg == nil {
		return hidden
	}
	for _, w := range c.Workspaces {
		if !w.Private {
			continue
		}
		for _, s := range w.Sites {
			hidden[s] = true
		}
	}
	hiddenGroups := map[string]bool{}
	for _, s := range reg.Sites {
		if s.Private {
			hidden[s.Name] = true
		}
		if hidden[s.Name] && s.Group != "" && s.GroupSubdomain == "" {
			hiddenGroups[s.Group] = true
		}
	}
	for _, s := range reg.Sites {
		if s.GroupSubdomain != "" && hiddenGroups[s.Group] {
			hidden[s.Name] = true
		}
	}
	return hidden
}

// VisibleWorkspaceNames is WorkspaceNames minus the private workspaces while
// streaming mode is on.
func (c *GlobalConfig) VisibleWorkspaceNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Workspaces))
	for _, w := range c.Workspaces {
		if c.UI.StreamingMode && w.Private {
			continue
		}
		names = append(names, w.Name)
	}
	return names
}

// SetWorkspacePrivate flags or clears a workspace as private.
func SetWorkspacePrivate(name string, private bool) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		i := c.workspaceIndex(name)
		if i < 0 {
			return fmt.Errorf("%q: %w", name, ErrWorkspaceNotFound)
		}
		c.Workspaces[i].Private = private
		return nil
	})
}

// SetStreamingMode turns streaming mode on or off.
func SetStreamingMode(on bool) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		c.UI.StreamingMode = on
		return nil
	})
}

// SetStreamingAuto opts in or out of turning streaming mode on during a share.
func SetStreamingAuto(on bool) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		c.UI.StreamingAuto = on
		return nil
	})
}
