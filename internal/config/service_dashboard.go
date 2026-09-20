package config

// ServiceDashboard resolves where a service's dashboard lives.
//
// An installed service records what its preset declared at the moment it was
// installed, so one installed before its preset gained a dashboard carries none
// of its own. That is not a rare case: the store reaches every install within a
// day whatever version of lerd is running, and a definition is published per
// schema, so a binary too old to serve a dashboard is handed a definition
// without one and keeps that record after it is upgraded. Reading the preset
// when the record is silent is what lets the upgrade take effect, and it matches
// how the proxy flags beside it are already resolved.
//
// A record with a dashboard of its own always wins, so a user who edited theirs
// keeps it.
func ServiceDashboard(svc *CustomService) string {
	if svc == nil {
		return ""
	}
	if svc.Dashboard != "" {
		return svc.Dashboard
	}
	if svc.Preset == "" {
		return ""
	}
	p, err := LoadPreset(svc.Preset)
	if err != nil {
		return ""
	}
	return p.Dashboard
}
