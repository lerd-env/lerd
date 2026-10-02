//go:build linux

package cli

// installAutostart is a no-op on Linux — autostart is opt-in via
// `lerd autostart enable` or the web UI toggle.
func installAutostart() {}

// syncLoginAutostart has nothing to do here: login start rides on the service
// units ApplyAutostart already enables or disables.
func syncLoginAutostart(bool) {}
