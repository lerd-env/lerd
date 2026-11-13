package siteops

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/plugins"
)

// pageVhostsFn rewrites every vhost serving a site, its worktrees included,
// and pageReloadFn waits until nginx serves them, so a page loaded right after
// the switch already has the script; both swapped out in tests.
var (
	pageVhostsFn = func(s config.Site) error {
		if ServedByFPM(s) {
			return RegenerateFPMSiteVhosts(s)
		}
		return RegenerateSiteVhost(&s, s.PrimaryDomain())
	}
	pageReloadFn = nginx.ReloadAndSettle
)

// SetPagePlugin switches a plugin's page script on or off for a site's pages
// and rewrites its vhosts so the pages gain or lose it at once. It is the one
// path the CLI and the dashboard share.
func SetPagePlugin(site *config.Site, plugin string, on bool) error {
	if on {
		p, err := plugins.Find(plugin)
		if err != nil {
			return err
		}
		if p.UI == nil || p.UI.PageScript == "" {
			return fmt.Errorf("plugin %s has no page script", plugin)
		}
	}
	if err := config.SetSitePagePlugin(site.Name, plugin, on); err != nil {
		return err
	}
	updated, err := config.FindSite(site.Name)
	if err != nil {
		return err
	}
	if err := pageVhostsFn(*updated); err != nil {
		return err
	}
	return pageReloadFn()
}

// SetOverviewCard switches a plugin's card on or off for a site's Overview.
// Like a page script, only a plugin that has a card can be switched on, and
// switching off always works so a removed plugin's setting can be cleared.
func SetOverviewCard(site *config.Site, plugin string, on bool) error {
	if on {
		p, err := plugins.Find(plugin)
		if err != nil {
			return err
		}
		if p.UI == nil || p.UI.OverviewCard == nil {
			return fmt.Errorf("plugin %s has no overview card", plugin)
		}
	}
	return config.SetSiteOverviewCard(site.Name, plugin, on)
}

// SetSiteTab shows or hides a plugin's tab on a site. Tabs are on everywhere
// until hidden, so hiding needs a plugin with a tab and showing always works,
// which clears the setting of a removed plugin.
func SetSiteTab(site *config.Site, plugin string, on bool) error {
	if !on {
		p, err := plugins.Find(plugin)
		if err != nil {
			return err
		}
		if p.UI == nil || p.UI.SiteTab == nil {
			return fmt.Errorf("plugin %s has no site tab", plugin)
		}
	}
	return config.SetSiteTabHidden(site.Name, plugin, !on)
}
