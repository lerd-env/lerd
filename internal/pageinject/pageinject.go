// Package pageinject decides which scripts lerd adds to a site's HTML pages,
// the browser log reporter and page plugins, so every web server lerd runs
// injects the same tags.
package pageinject

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// BrowserLogs reports whether the site's pages report their console errors:
// debug capture on and browser logs on for the site.
func BrowserLogs(site config.Site) bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg.IsDumpsEnabled() && config.BrowserLogsFor(site).Enabled
}

// Plugins lists the site's page plugins that are installed and ship a page
// script; one switched on but since removed is skipped.
func Plugins(site config.Site) []string {
	var out []string
	for _, name := range site.PagePlugins {
		if plugins.HasPageScript(name) {
			out = append(out, name)
		}
	}
	return out
}

// Tags is what goes in front of </head>, rid standing for the id of the
// request that served the page so the browser's events link to it; "" when
// the site injects nothing.
func Tags(site config.Site, rid string) string {
	tags := ""
	if BrowserLogs(site) {
		tags += fmt.Sprintf(`<script src="%s.js" data-rid="%s"></script>`, config.BrowserLogsPath, rid)
	}
	for _, name := range Plugins(site) {
		tags += fmt.Sprintf(`<script src="%s%s/page.js" defer></script>`, plugins.PagePath, name)
	}
	return tags
}
