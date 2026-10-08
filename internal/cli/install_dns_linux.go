//go:build linux

package cli

import (
	"github.com/geodro/lerd/internal/dns"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/services"
)

// removeLegacyDNSService retires the dnsmasq container that ran lerd-dns
// before the built-in server, so it stops holding port 5300. Its image goes
// too, since nothing else uses it. Reports whether there was one.
func removeLegacyDNSService() bool {
	if !podman.QuadletInstalled(dnsUnit) {
		return false
	}
	_ = services.Mgr.Stop(dnsUnit)
	_ = services.Mgr.RemoveContainerUnit(dnsUnit)
	podman.Cmd("rm", "-f", dnsUnit).Run()               //nolint:errcheck
	podman.Cmd("rmi", "-f", "lerd-dnsmasq:local").Run() //nolint:errcheck
	return true
}

// prepDNSForRollback deletes the lerd-dns service unit, which would otherwise
// outrank the quadlet an older lerd writes for its dnsmasq container and run a
// dns-serve command that binary does not have.
func prepDNSForRollback() { _ = services.Mgr.RemoveServiceUnit(dnsUnit) }

func teardownDNSResolver() {
	// Only when lerd actually wrote resolver config. install.go calls this on
	// every run where DNS is off, not just on a true->false flip, so an
	// unconditional teardown would revert interfaces and restart NetworkManager on
	// every `lerd install` for someone who never let lerd near their resolver.
	if !dnsResolverConfigured() {
		return
	}
	// Announced with the lock glyph: the removals run as root. They are granted in
	// the sudoers drop-in so they do not prompt, but the header keeps the teardown
	// visible in the output.
	feedback.Sudo("Removing DNS configuration")
	dnsTeardown()
}

// Seams so tests can drive the disable path without shelling out to sudo or
// depending on what the test host happens to have installed.
var (
	dnsTeardown           = dns.Teardown
	dnsResolverConfigured = dns.ResolverConfigured
)
