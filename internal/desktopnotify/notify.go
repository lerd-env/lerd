// Package desktopnotify posts native desktop notifications from the daemon.
// On Linux it calls org.freedesktop.Notifications on the session bus using the
// godbus dependency lerd already ships; other platforms are no-ops for now.
package desktopnotify

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed icon.png
var iconPNG []byte

var (
	iconOnce sync.Once
	iconFile string
)

// IconPath materializes the bundled lerd icon to a stable cache path and returns
// its absolute path. Notification daemons that ignore themed names still show
// the logo this way. Returns "" on failure, letting the daemon draw its default.
func IconPath() string {
	iconOnce.Do(func() {
		base, err := os.UserCacheDir()
		if err != nil {
			return
		}
		dir := filepath.Join(base, "lerd")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
		p := filepath.Join(dir, "notify-icon.png")
		if _, statErr := os.Stat(p); statErr != nil {
			if err := os.WriteFile(p, iconPNG, 0o644); err != nil {
				return
			}
		}
		iconFile = p
	})
	return iconFile
}

// Urgency maps to the org.freedesktop.Notifications "urgency" hint byte.
type Urgency byte

const (
	UrgencyLow      Urgency = 0
	UrgencyNormal   Urgency = 1
	UrgencyCritical Urgency = 2
)

// Request is a single native notification to show.
type Request struct {
	AppName string
	Icon    string // themed icon name or absolute path; "" shows the daemon default
	Summary string
	Body    string
	Urgency Urgency
	Route   string // dashboard route to open on click; "" makes the popup inert
}

// appSchemeURL is the lerd:// deep link the desktop app claims, used when it is
// installed so a click focuses its window at the right route.
func appSchemeURL(route string) string {
	return "lerd://open/" + strings.TrimPrefix(route, "/")
}

// dashboardURL is the loopback dashboard URL a click falls back to when the
// desktop app is not installed, opened in the default browser.
func dashboardURL(route string) string {
	return "http://127.0.0.1:7073/" + strings.TrimPrefix(route, "/")
}

// UrgencyFromString maps lerd's notification urgency strings to the DBus hint.
// Unknown or empty values are Normal, matching the Web Push default.
func UrgencyFromString(s string) Urgency {
	switch s {
	case "critical", "high":
		return UrgencyCritical
	case "low":
		return UrgencyLow
	default:
		return UrgencyNormal
	}
}
