//go:build linux

package cli

import "testing"

// Disabling DNS must remove the resolver plumbing, not just stop the container.
// Leaving it behind pointed the dispatcher and the interface routes at a dnsmasq
// that is no longer running, and stranded the lerd0 offline link on the host with
// nothing maintaining it and no obvious way for the user to get rid of it. The
// macOS path already tore its /etc/resolver files down here; Linux did not.
func TestTeardownDNS_removesResolverPlumbing(t *testing.T) {
	origTeardown, origConfigured := dnsTeardown, dnsResolverConfigured
	t.Cleanup(func() { dnsTeardown, dnsResolverConfigured = origTeardown, origConfigured })

	called := false
	dnsTeardown = func() { called = true }
	dnsResolverConfigured = func() bool { return true } // lerd did write resolver config

	teardownDNS()

	if !called {
		t.Error("teardownDNS must tear down the resolver config so lerd0 and the dispatcher don't outlive `lerd dns:disable`")
	}
}

// install.go calls teardownDNS on every run where DNS is off, not only on a
// true->false flip. Tearing down unconditionally reverts interfaces and restarts
// NetworkManager on every `lerd install` for someone who never let lerd manage
// DNS, so it has to be gated on lerd having actually written resolver config.
func TestTeardownDNS_skipsWhenLerdNeverConfiguredTheResolver(t *testing.T) {
	origTeardown, origConfigured := dnsTeardown, dnsResolverConfigured
	t.Cleanup(func() { dnsTeardown, dnsResolverConfigured = origTeardown, origConfigured })

	called := false
	dnsTeardown = func() { called = true }
	dnsResolverConfigured = func() bool { return false } // lerd never touched the resolver

	teardownDNS()

	if called {
		t.Error("teardownDNS must not revert interfaces and restart NetworkManager on a host where lerd never wrote resolver config")
	}
}
