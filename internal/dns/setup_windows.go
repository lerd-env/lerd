//go:build windows

package dns

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// Windows has no sudo. The .<tld> namespace is routed to the lerd-dns dnsmasq
// through a DNS Client NRPT rule, which needs an elevated shell and can only
// name a server, not a port, so dnsmasq must answer on 127.0.0.1:53.
var (
	sudoersProbeCommand = "powershell.exe"
	sudoProbeArgs       = []string{"-NoProfile", "-Command", "exit 0"}
)

func readUpstreamDNS() []string {
	if servers := configuredUpstreamDNS(); len(servers) > 0 {
		return servers
	}
	return adapterDNSServers()
}

// defaultUpstreamFallback is nil: the Podman machine seeds the container's
// resolv.conf from the host, as on macOS.
func defaultUpstreamFallback() []string { return nil }

func powershell(script string) ([]byte, error) {
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
}

// adapterDNSServers lists the IPv4 DNS servers of every connected adapter.
func adapterDNSServers() []string {
	out, err := powershell(`Get-DnsClientServerAddress -AddressFamily IPv4 | ForEach-Object { $_.ServerAddresses } | Where-Object { $_ -ne '127.0.0.1' } | Select-Object -Unique`)
	if err != nil {
		return nil
	}
	var servers []string
	for _, l := range strings.Fields(string(out)) {
		servers = append(servers, l)
	}
	return servers
}

func nrptNamespace() string { return "." + ConfiguredTLD() }

// ConfigureResolver adds the NRPT rule sending .<tld> to 127.0.0.1.
func ConfigureResolver() error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if cfg != nil && !cfg.DNS.Enabled {
		return nil
	}
	ns := nrptNamespace()
	script := fmt.Sprintf(`if (-not (Get-DnsClientNrptRule | Where-Object { $_.Namespace -contains '%[1]s' })) { Add-DnsClientNrptRule -Namespace '%[1]s' -NameServers '127.0.0.1' -Comment 'lerd' }`, ns)
	if out, err := powershell(script); err != nil {
		return fmt.Errorf("configuring NRPT rule for %s (needs an elevated shell): %w: %s", ns, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Teardown removes every NRPT rule lerd created, matched by its comment.
func Teardown() {
	_, _ = powershell(`Get-DnsClientNrptRule | Where-Object { $_.Comment -eq 'lerd' } | Remove-DnsClientNrptRule -Force`)
}

// InstallSudoers is a no-op: there is no sudoers on Windows.
func InstallSudoers() error { return nil }

func ReadContainerDNS() []string { return nil }

func ReadUpstreamDNS() []string { return readUpstreamDNS() }

func ResolverHint() string {
	return "run 'lerd install' from an elevated shell to reconfigure DNS"
}

func NoteNixOSOwnsResolver() {}

// RepairPossible is true only when the process is elevated, since writing NRPT
// rules needs administrator rights.
func RepairPossible() bool {
	f, err := os.Open(`\.\PHYSICALDRIVE0`)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// NRPT names servers, not ports, so lerd-dns has to sit on the standard one.
func init() { dnsPort = 53 }
