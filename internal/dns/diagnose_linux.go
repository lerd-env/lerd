package dns

import (
	"fmt"
	"os"
)

// routesThroughResolved: the host routes .tld through systemd-resolved
// interfaces and lerd's dummy link, which diagnose checks as rungs 6 and 6b.
const routesThroughResolved = true

// findListenerCmd returns the shell command the user can run to identify the
// process bound to a TCP port.
func findListenerCmd(port int) string {
	return fmt.Sprintf("ss -tlnp sport = :%d", port)
}

// defaultResolverHookup reports how .test is wired into the system resolver.
//
// Ordered, not a map: the NetworkManager path installs both the dispatcher and
// the lerd0 link unit, so a map's random iteration would report either one at
// random from run to run. First match wins, most specific first.
func defaultResolverHookup() (string, bool, string) {
	for _, h := range []struct{ kind, path string }{
		{nmDispatcherKind, "/etc/NetworkManager/dispatcher.d/99-lerd-dns"},
		{nmDnsmasqKind, "/etc/NetworkManager/dnsmasq.d/lerd.conf"},
		// No NetworkManager: lerd0 alone carries .tld, so its unit is the hookup.
		{resolvedLinkKind, lerdLinkUnit},
		// Last: a host that has not re-run setup since the link landed still
		// resolves through this, and reporting "no hookup" at it would be a lie.
		{resolvedDropinKind, "/etc/systemd/resolved.conf.d/lerd.conf"},
	} {
		if _, err := os.Stat(h.path); err == nil {
			return h.kind, true, h.path
		}
	}
	return "", false, ""
}
