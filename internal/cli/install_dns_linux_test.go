//go:build linux

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// isolateState points the lerd state dirs at temp dirs for the duration of a
// test. teardownDNS deletes the lerd-dns quadlet, which without this lands on the
// developer's own install.
func isolateState(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

// Disabling DNS must remove the resolver plumbing, not just stop the container.
// Leaving it behind pointed the dispatcher and the interface routes at a dnsmasq
// that is no longer running, and stranded the lerd0 offline link on the host with
// nothing maintaining it and no obvious way for the user to get rid of it. The
// macOS path already tore its /etc/resolver files down here; Linux did not.
func TestTeardownDNS_removesResolverPlumbing(t *testing.T) {
	isolateState(t)
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
	isolateState(t)
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

// A lerd-dns.service in the user unit dir outranks the generator output of the
// quadlet an older lerd writes, so a rollback that left it behind would run
// `lerd dns-serve` on a binary that has no such command, and DNS would die.
func TestPrepDNSForRollback_removesTheServiceUnit(t *testing.T) {
	isolateState(t)
	path := filepath.Join(config.SystemdUserDir(), "lerd-dns.service")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(dnsServiceContent("/bin/lerd", "127.0.0.1")), 0644); err != nil {
		t.Fatal(err)
	}

	prepDNSForRollback()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("lerd-dns.service must be gone before the older binary installs, stat err = %v", err)
	}
}

// The old container keeps .test resolving until its replacement is on disk:
// when writing the new unit fails, the install stops with DNS still working.
func TestInstallDNSService_keepsTheOldServerWhenTheNewUnitFails(t *testing.T) {
	isolateState(t)
	swapMgr(t, &fakeServiceMgr{writeErr: errors.New("disk full")})
	removed := false
	prev := removeLegacyDNS
	removeLegacyDNS = func() bool { removed = true; return true }
	t.Cleanup(func() { removeLegacyDNS = prev })

	if err := installDNSService(); err == nil {
		t.Fatal("installDNSService() = nil, want the write error")
	}
	if removed {
		t.Error("the working dnsmasq container was removed before its replacement was written")
	}
}

// systemd has to have the new unit loaded before the old container goes, or
// a failed reload leaves nothing serving the lerd TLD.
func TestInstallDNSService_keepsTheOldServerWhenTheReloadFails(t *testing.T) {
	isolateState(t)
	swapMgr(t, &fakeServiceMgr{reloadErr: errors.New("bus unavailable")})
	removed := false
	prev := removeLegacyDNS
	removeLegacyDNS = func() bool { removed = true; return true }
	t.Cleanup(func() { removeLegacyDNS = prev })

	if err := installDNSService(); err == nil {
		t.Fatal("installDNSService() = nil, want the reload error")
	}
	if removed {
		t.Error("the working dnsmasq container was removed before systemd loaded its replacement")
	}
}
