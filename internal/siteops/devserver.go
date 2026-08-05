package siteops

import "github.com/geodro/lerd/internal/config"

// RefreshDevServers realigns a site's generated dev server config, and the dev
// server running on it, with the addresses the site now answers on. A dev server
// reads its origin and its allowed hosts once, at start, so securing a site or
// moving its domains leaves it serving an address that has gone.
//
// The mechanism lives in the cli package, which wires the real implementation in
// (see internal/cli/devserver_wire.go); the default is a no-op so a binary that
// does not link it, and every test in this package, behaves as before.
var RefreshDevServers = func(*config.Site) {}

// StopSiteShares closes any share listener the site still has open. Unlinking
// drops the registry entry, so a listener left bound afterwards can never be
// found again by name and survives until the daemon exits. Wired by the cli
// package, which owns the proxies.
var StopSiteShares = func(siteName string) {}
