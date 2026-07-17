//go:build linux

package cli

import "testing"

// Disabling DNS must remove the resolver plumbing, not just stop the container.
// Leaving it behind pointed the dispatcher and the interface routes at a dnsmasq
// that is no longer running, and stranded the lerd0 offline link on the host with
// nothing maintaining it and no obvious way for the user to get rid of it. The
// macOS path already tore its /etc/resolver files down here; Linux did not.
func TestTeardownDNS_removesResolverPlumbing(t *testing.T) {
	orig := dnsTeardown
	t.Cleanup(func() { dnsTeardown = orig })

	called := false
	dnsTeardown = func() { called = true }

	teardownDNS()

	if !called {
		t.Error("teardownDNS must tear down the resolver config so lerd0 and the dispatcher don't outlive `lerd dns:disable`")
	}
}
