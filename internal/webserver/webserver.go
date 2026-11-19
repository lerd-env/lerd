// Package webserver is the one door to the web server that serves lerd's
// sites. Callers describe a site; the backend decides how to serve it.
package webserver

import (
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
}

type nginxBackend struct{}

func (nginxBackend) ApplySite(site config.Site) error { return nginx.ApplySite(site) }
func (nginxBackend) RemoveSite(domain string) error   { return nginx.RemoveVhost(domain) }

// current is nginx until a second backend and the switch to it exist.
var current Backend = nginxBackend{}

func ApplySite(site config.Site) error { return current.ApplySite(site) }

func RemoveSite(domain string) error { return current.RemoveSite(domain) }
