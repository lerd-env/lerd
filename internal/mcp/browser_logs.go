package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/geodro/lerd/internal/browserlogs"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

// execBrowserEvents returns what the browser logs script reported for a
// site, grouped per page view: JavaScript errors, unhandled rejections,
// console messages, failed requests and resources, configured DOM events and
// the page views themselves. types narrows it, and hint says why it is empty.
func execBrowserEvents(args map[string]any) (any, *rpcError) {
	var types []string
	if raw, ok := args["types"].([]any); ok {
		for _, v := range raw {
			t, _ := v.(string)
			if !slices.Contains(browserlogs.EventTypes, t) {
				return toolErr(fmt.Sprintf("unknown type %q (want any of %v)", t, browserlogs.EventTypes)), nil
			}
			types = append(types, t)
		}
	}
	limit := ""
	if v, ok := args["limit"]; ok {
		limit = fmt.Sprintf("%v", v)
	}
	path := queryPath("/api/dumps", [][2]string{
		{"kind", dumps.KindBrowser},
		{"site", strArg(args, "site")},
		{"branch", strArg(args, "branch")},
		{"since", strArg(args, "since")},
		{"limit", limit},
	})
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	var events []dumps.Event
	if err := json.Unmarshal(body, &events); err != nil {
		return toolErr("decoding events: " + err.Error()), nil
	}
	summary := browserlogs.Summarize(events, types)
	cfg, _ := config.LoadGlobal()
	out := map[string]any{
		"debug_enabled": cfg != nil && cfg.IsDumpsEnabled(),
		"counts":        summary.Counts,
		"page_views":    summary.PageViews,
	}
	var site *config.Site
	if ref := strArg(args, "site"); ref != "" {
		if site, _ = config.FindSiteByRef(ref); site != nil {
			settings := config.BrowserLogsFor(*site)
			out["site_enabled"] = settings.Enabled
			out["site_settings"] = settings
		}
	}
	if hint := browserEventsHint(out["debug_enabled"].(bool), site, len(summary.PageViews), len(types) > 0); hint != "" {
		out["hint"] = hint
	}
	b, _ := json.Marshal(out)
	return toolOK(string(b)), nil
}

// browserEventsHint explains an empty answer, so no events is never read as a
// page that threw nothing while capture was not even running.
func browserEventsHint(debug bool, site *config.Site, views int, filtered bool) string {
	switch {
	case site != nil && !browserlogs.Capturable(*site):
		return "This site's pages are not covered by browser logs (FrankenPHP, paused or a sleeping host-proxy site)."
	case site != nil && !config.BrowserLogsFor(*site).Enabled:
		return "Browser logs are off for this site. Turn them on with browser_toggle (site, enable: true), then load a page of the site."
	case !debug:
		return "Debug capture is off, so nothing is recorded. Turn it on with dumps_toggle (enable: true), then load a page of the site."
	case views == 0 && filtered:
		return "Nothing of the requested types. Drop types to see every page view and what happened on it."
	case views == 0:
		return "No events yet. Load or reload a page of the site in a browser; every page view is recorded, and errors on it are listed under it."
	}
	return ""
}

// execBrowserLogsToggle turns capture on or off for one site, and says
// when debug capture is off too, since the script needs both.
func execBrowserLogsToggle(args map[string]any) (any, *rpcError) {
	site, err := config.FindSiteByRef(strArg(args, "site"))
	if err != nil || site == nil {
		return toolErr(`"site" must name a linked site`), nil
	}
	enable, ok := args["enable"].(bool)
	if !ok {
		return toolErr(`"enable" is required (true or false)`), nil
	}
	res, err := browserlogs.SetSite(*site, enable)
	if err != nil {
		return toolErr("toggle failed: " + err.Error()), nil
	}
	cfg, _ := config.LoadGlobal()
	debug := cfg != nil && cfg.IsDumpsEnabled()
	out := map[string]any{"site": site.Name, "enabled": res.Enabled, "no_change": res.NoChange, "debug_enabled": debug}
	if enable && !debug {
		out["hint"] = "Debug capture is off, so the site's pages get the script once dumps_toggle (enable: true) turns it on."
	}
	b, _ := json.Marshal(out)
	return toolOK(string(b)), nil
}

// execBrowserPresets lists the store's event presets for a site with what was
// detected and on, or switches the one named by preset on (enable true) or off.
func execBrowserPresets(args map[string]any) (any, *rpcError) {
	site, err := config.FindSiteByRef(strArg(args, "site"))
	if err != nil || site == nil {
		return toolErr(`"site" must name a linked site`), nil
	}
	if name := strArg(args, "preset"); name != "" {
		add, ok := args["enable"].(bool)
		if !ok {
			return toolErr(`"enable" is required with "preset" (true adds, false removes)`), nil
		}
		if err := browserlogs.SetPreset(*site, name, add); err != nil {
			return toolErr(err.Error()), nil
		}
		site, _ = config.FindSite(site.Name)
	}
	b, _ := json.Marshal(browserlogs.Presets(*site))
	return toolOK(string(b)), nil
}
