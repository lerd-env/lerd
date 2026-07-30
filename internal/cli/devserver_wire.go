package cli

import "github.com/geodro/lerd/internal/siteops"

// Wire the dev server refresh to the siteops paths that move a site's addresses,
// securing it and changing its domains, which the CLI, the UI and MCP all share.
// The mechanism lives in this package, so the hook is filled in from here.
func init() { siteops.RefreshDevServers = RefreshDevServers }
