// Package webserver is the one door to the web server that serves lerd's
// sites. Callers describe a site; the backend decides how to serve it.
package webserver

import (
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
)

// Backend is a web server lerd can serve sites through.
type Backend interface {
	// ApplySite writes what serves the site's own domain in its current kind
	// and scheme.
	ApplySite(site config.Site) error
	// RemoveSite stops serving domain.
	RemoveSite(domain string) error
	// Reload makes the running server pick up what was written, returning
	// ErrNotRunning when it is down; it reads the config when it starts.
	Reload() error
	// ReloadWithRetry retries Reload while the server is briefly unavailable.
	ReloadWithRetry(timeout time.Duration) error
	// ReloadAndSettle reloads and returns once requests see the new config.
	ReloadAndSettle() error
	// Test validates the written config, returning the server's own output.
	Test() (string, error)
	// ServesVhostWrittenAt reports whether a config written at mod is live.
	ServesVhostWrittenAt(mod time.Time) bool
	// ApplyWorktree serves a PHP worktree of a site on its own domain.
	ApplyWorktree(domain, path, phpVersion, parentDomain, siteName, branch string, secured bool) error
	// ApplyHostProxyWorktree serves a host-proxy site's worktree from port.
	ApplyHostProxyWorktree(domain, path, parentDomain string, port int, backendSSL, secured bool) error
	// ApplyPausedSite and ApplyPausedWorktree serve the paused landing page.
	ApplyPausedSite(site config.Site) error
	ApplyPausedWorktree(domain, certDomain, pausedDir string, secured bool) error
	// ApplyWakingSite holds the site's requests at WakeHoldPath while it wakes.
	ApplyWakingSite(site config.Site) error
	// Drift compares what serves the site with what lerd would write now.
	Drift(site config.Site) (DriftReport, error)
	// RegenerateSite rewrites a drifted site, leaving paused and waking pages.
	RegenerateSite(site config.Site) error
	// Repair fixes vhosts that would stop the server starting, before it starts.
	Repair() []Repair
}

// DriftReport and Repair describe what Drift and Repair found.
type (
	DriftReport = nginx.VhostDriftReport
	Repair      = nginx.VhostRepair
)

// WakeHoldPath is where a waking site's requests are held in lerd-ui until
// the site is back, then replayed whole.
const WakeHoldPath = nginx.WakeHoldPath

// ErrNotRunning reports that the web server is down, so there was nothing to
// reload; whoever starts it next reads the config already written.
var ErrNotRunning = nginx.ErrNotRunning

type nginxBackend struct{}

func (nginxBackend) ApplySite(site config.Site) error { return nginx.ApplySite(site) }
func (nginxBackend) RemoveSite(domain string) error   { return nginx.RemoveVhost(domain) }
func (nginxBackend) Reload() error                    { return nginx.Reload() }
func (nginxBackend) ReloadWithRetry(d time.Duration) error {
	return nginx.ReloadWithRetry(d)
}
func (nginxBackend) ReloadAndSettle() error                  { return nginx.ReloadAndSettle() }
func (nginxBackend) Test() (string, error)                   { return nginx.Test() }
func (nginxBackend) ServesVhostWrittenAt(mod time.Time) bool { return nginx.ServesVhostWrittenAt(mod) }
func (nginxBackend) ApplyWorktree(domain, path, phpVersion, parentDomain, siteName, branch string, secured bool) error {
	return nginx.GenerateWorktreeVhostFor(domain, path, phpVersion, parentDomain, siteName, branch, secured)
}
func (nginxBackend) ApplyHostProxyWorktree(domain, path, parentDomain string, port int, backendSSL, secured bool) error {
	return nginx.GenerateWorktreeHostProxyVhostFor(domain, path, parentDomain, port, backendSSL, secured)
}
func (nginxBackend) ApplyPausedSite(site config.Site) error { return nginx.GeneratePausedVhost(site) }
func (nginxBackend) ApplyPausedWorktree(domain, certDomain, pausedDir string, secured bool) error {
	return nginx.GeneratePausedWorktreeVhost(domain, certDomain, pausedDir, secured)
}
func (nginxBackend) ApplyWakingSite(site config.Site) error      { return nginx.GenerateWakingVhost(site) }
func (nginxBackend) Drift(site config.Site) (DriftReport, error) { return nginx.VhostDrift(site) }
func (nginxBackend) RegenerateSite(site config.Site) error       { return nginx.RegenerateVhost(site) }
func (nginxBackend) Repair() []Repair                            { return nginx.RepairVhosts() }

// current is nginx until a second backend and the switch to it exist.
var current Backend = nginxBackend{}

func ApplySite(site config.Site) error { return current.ApplySite(site) }

func RemoveSite(domain string) error { return current.RemoveSite(domain) }

func Reload() error { return current.Reload() }

func ReloadWithRetry(timeout time.Duration) error { return current.ReloadWithRetry(timeout) }

func ReloadAndSettle() error { return current.ReloadAndSettle() }

func Test() (string, error) { return current.Test() }

func ServesVhostWrittenAt(mod time.Time) bool { return current.ServesVhostWrittenAt(mod) }

func ApplyWorktree(domain, path, phpVersion, parentDomain, siteName, branch string, secured bool) error {
	return current.ApplyWorktree(domain, path, phpVersion, parentDomain, siteName, branch, secured)
}

func ApplyHostProxyWorktree(domain, path, parentDomain string, port int, backendSSL, secured bool) error {
	return current.ApplyHostProxyWorktree(domain, path, parentDomain, port, backendSSL, secured)
}

func ApplyPausedSite(site config.Site) error { return current.ApplyPausedSite(site) }

func ApplyPausedWorktree(domain, certDomain, pausedDir string, secured bool) error {
	return current.ApplyPausedWorktree(domain, certDomain, pausedDir, secured)
}

func ApplyWakingSite(site config.Site) error { return current.ApplyWakingSite(site) }

func Drift(site config.Site) (DriftReport, error) { return current.Drift(site) }

func RegenerateSite(site config.Site) error { return current.RegenerateSite(site) }

func RepairSites() []Repair { return current.Repair() }
