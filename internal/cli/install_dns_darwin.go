//go:build darwin

package cli

import "github.com/geodro/lerd/internal/dns"

// removeLegacyDNSService has nothing to do on macOS: the Homebrew dnsmasq
// lerd-dns ran before used the same plist, which installDNSService rewrites
// and the restart reloads.
func removeLegacyDNSService() bool { return false }

// prepDNSForRollback has nothing to do on macOS: every older lerd rewrites the
// lerd-dns plist itself on install.
func prepDNSForRollback() {}

// teardownDNSResolver removes the /etc/resolver files lerd wrote so a disabled
// setup doesn't leave a resolver pointing at a server that is gone.
func teardownDNSResolver() { dns.Teardown() }
