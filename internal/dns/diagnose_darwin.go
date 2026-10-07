package dns

import "fmt"

// routesThroughResolved is false: macOS has no systemd-resolved interfaces or
// dummy link to check.
const routesThroughResolved = false

// findListenerCmd returns the shell command the user can run to identify the
// process bound to a TCP port. macOS lacks ss(8), so it is lsof, which ships
// with the OS.
func findListenerCmd(port int) string {
	return fmt.Sprintf("lsof -nP -iTCP:%d -sTCP:LISTEN", port)
}

// defaultResolverHookup reports the one hookup macOS has.
func defaultResolverHookup() (string, bool, string) {
	return macOSKind, true, "/usr/local/etc/dnsmasq.d/lerd.conf"
}
