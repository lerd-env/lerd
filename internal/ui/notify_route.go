package ui

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

// debugRouteForContext returns the dashboard route a debug notification opens.
// Debug events belong to a site, so they land on that site's Debug tab; when no
// site can be resolved the sites list is the honest destination, since the
// global bridge view says nothing about the event that was clicked.
//
// The route names the lens as well as the tab. Without it the Debug tab opens
// on whichever lens was last looked at, so clicking a message notification
// could land on Queries and show nothing of what was clicked.
func debugRouteForContext(ctx dumps.Context, kind string) string {
	domain := debugSiteDomain(ctx)
	if domain == "" {
		return "#sites"
	}
	route := "#sites/" + domain + "/dumps"
	if lens := debugLensForKind(kind); lens != "" {
		route += "/" + lens
	}
	return route
}

// debugLensForKind maps a wire kind to the Debug lens that draws it. A kind
// with no lens of its own opens the tab as it was left.
func debugLensForKind(kind string) string {
	switch kind {
	case dumps.KindDump:
		return "dumps"
	case dumps.KindQuery:
		return "queries"
	case dumps.KindJob:
		return "jobs"
	case dumps.KindView:
		return "views"
	case dumps.KindMail:
		return "mail"
	case dumps.KindCache:
		return "cache"
	case dumps.KindEvent:
		return "events"
	case dumps.KindHTTP:
		return "http"
	case dumps.KindLog:
		return "logs"
	case dumps.KindException:
		return "exceptions"
	case dumps.KindMessage:
		return "messages"
	default:
		return ""
	}
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
