//go:build linux || darwin

package cli

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/services"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
)

const dnsUnit = "lerd-dns"

// dnsServiceContent is the unit for lerd's built-in DNS server. services.Mgr
// writes it as a systemd user service on Linux and a launchd plist on macOS.
// StartLimitIntervalSec=0 because the NetworkManager dispatcher restarts it on
// every interface event, and a resume outruns systemd's default rate limit.
func dnsServiceContent(exe, listen string) string {
	return "[Unit]\nDescription=Lerd DNS\nAfter=network.target\nStartLimitIntervalSec=0\n\n" +
		"[Service]\nExecStart=" + exe + " dns-serve --listen " + listen + "\nRestart=always\n\n" +
		"[Install]\nWantedBy=default.target\n"
}

// dnsListenHost is where lerd-dns listens. Where it answers the LAN itself
// (macOS, as its dnsmasq did) it takes every interface; elsewhere it stays on
// loopback and lan:expose relays the LAN to it through lerd-dns-forwarder.
func dnsListenHost() string {
	if lerdDNSBindsLANPort {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

// removeLegacyDNS is removeLegacyDNSService behind a seam, so tests never run
// podman against the developer's own lerd-dns container.
var removeLegacyDNS = removeLegacyDNSService

// installDNSService writes the lerd-dns unit, then replaces whatever ran DNS
// before it. The unit goes first so a failed write leaves the old server up.
func installDNSService() error {
	if _, err := services.Mgr.WriteServiceUnitIfChanged(dnsUnit, dnsServiceContent(config.LerdBinary(), dnsListenHost())); err != nil {
		return err
	}
	replaced := removeLegacyDNS()
	if err := services.Mgr.DaemonReload(); err != nil {
		return err
	}
	if lerdSystemd.IsAutostartEnabled() {
		_ = services.Mgr.Enable(dnsUnit)
	}
	// The container just removed was answering .test, and every query when the
	// resolver routes ~. here, so start its replacement before the image pulls.
	if replaced {
		_ = services.Mgr.Start(dnsUnit)
	}
	return nil
}

// teardownDNS stops lerd-dns and removes its unit, then the resolver config
// pointing at it. Called from runInstall whenever DNS is off; safe to call
// when nothing is installed.
func teardownDNS() {
	_ = services.Mgr.Stop(dnsUnit)
	_ = services.Mgr.RemoveServiceUnit(dnsUnit)
	_ = removeLegacyDNS()
	_ = services.Mgr.DaemonReload()
	teardownDNSResolver()
}
