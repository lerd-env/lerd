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

// installDNSService writes the lerd-dns unit, replacing whatever ran DNS
// before it, and reports whether the unit changed.
func installDNSService() (bool, error) {
	replaced := removeLegacyDNSService()
	changed, err := services.Mgr.WriteServiceUnitIfChanged(dnsUnit, dnsServiceContent(config.LerdBinary(), dnsListenHost()))
	if err != nil {
		return false, err
	}
	if err := services.Mgr.DaemonReload(); err != nil {
		return changed, err
	}
	if lerdSystemd.IsAutostartEnabled() {
		_ = services.Mgr.Enable(dnsUnit)
	}
	// The container just removed was answering .test, and every query when the
	// resolver routes ~. here, so start its replacement before the image pulls.
	if replaced {
		_ = services.Mgr.Start(dnsUnit)
	}
	return changed, nil
}

// teardownDNS stops lerd-dns and removes its unit, then the resolver config
// pointing at it. Called from runInstall whenever DNS is off; safe to call
// when nothing is installed.
func teardownDNS() {
	_ = services.Mgr.Stop(dnsUnit)
	_ = services.Mgr.RemoveServiceUnit(dnsUnit)
	_ = removeLegacyDNSService()
	_ = services.Mgr.DaemonReload()
	teardownDNSResolver()
}
