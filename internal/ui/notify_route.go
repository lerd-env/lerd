package ui

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

// debugRouteForContext returns the dashboard route a debug notification opens.
// Debug events belong to a site, so they land on that site's Debug tab; when no
// site can be resolved the sites list is the honest destination, since the
// global bridge view says nothing about the event that was clicked.
func debugRouteForContext(ctx dumps.Context) string {
	if domain := debugSiteDomain(ctx); domain != "" {
		return "#sites/" + domain + "/dumps"
	}
	return "#sites"
}

// debugSiteDomain resolves an event context to the primary domain the Sites tab
// is keyed by, preferring the site the bridge tagged and falling back to the
// request domain, which survives even when LERD_SITE never reached the process.
func debugSiteDomain(ctx dumps.Context) string {
	if ctx.Site != "" {
		return siteDomainForRoute(ctx.Site)
	}
	if ctx.Domain != "" {
		if s, err := config.FindSiteByDomain(ctx.Domain); err == nil && s != nil {
			return s.PrimaryDomain()
		}
	}
	return ""
}
