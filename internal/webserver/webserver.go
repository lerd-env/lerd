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
}

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

// current is nginx until a second backend and the switch to it exist.
var current Backend = nginxBackend{}

func ApplySite(site config.Site) error { return current.ApplySite(site) }

func RemoveSite(domain string) error { return current.RemoveSite(domain) }

func Reload() error { return current.Reload() }

func ReloadWithRetry(timeout time.Duration) error { return current.ReloadWithRetry(timeout) }

func ReloadAndSettle() error { return current.ReloadAndSettle() }

func Test() (string, error) { return current.Test() }

func ServesVhostWrittenAt(mod time.Time) bool { return current.ServesVhostWrittenAt(mod) }
