package config

import "fmt"

// The web servers lerd can serve sites with.
const (
	WebServerNginx = "nginx"
	WebServerGo    = "go"
)

// WebServerKind is the web server serving the sites. A value this binary does
// not know, as one written by a newer lerd, falls back to nginx, the server
// every lerd knows, and known reports false so the caller can say so.
func (c *GlobalConfig) WebServerKind() (kind string, known bool) {
	switch c.WebServer {
	case "", WebServerNginx:
		return WebServerNginx, true
	case WebServerGo:
		return WebServerGo, true
	}
	return WebServerNginx, false
}

// ValidateWebServer refuses a web server name lerd cannot run, naming it.
func ValidateWebServer(s string) error {
	switch s {
	case WebServerNginx, WebServerGo:
		return nil
	}
	return fmt.Errorf("unknown web server %q: lerd runs %q or %q", s, WebServerNginx, WebServerGo)
}
