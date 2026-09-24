package config

import "fmt"

// Streaming reports whether private workspaces are being hidden right now,
// which needs the feature enabled as well as the mode on.
func (c *GlobalConfig) Streaming() bool {
	return c != nil && c.UI.StreamingEnabled && c.UI.StreamingMode
}

// StreamingHidden returns the names of the sites that must stay off screen
// right now: nothing while streaming is off, otherwise PrivateSites.
func (c *GlobalConfig) StreamingHidden(reg *SiteRegistry) map[string]bool {
	if !c.Streaming() {
		return map[string]bool{}
	}
	return c.PrivateSites(reg)
}

// PrivateSites returns the sites streaming mode hides: every member of a
// private workspace, and the secondaries of a hidden group main, which render
// inside their main.
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
		if c.Streaming() && w.Private {
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

// SetStreamingEnabled turns the streaming feature on or off. Disabling it also
// clears the mode, so enabling it later never hides anything straight away.
func SetStreamingEnabled(on bool) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		c.UI.StreamingEnabled = on
		if !on {
			c.UI.StreamingMode = false
		}
		return nil
	})
}
