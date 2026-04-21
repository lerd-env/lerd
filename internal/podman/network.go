package podman

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// LerdULAv6Subnet is the deterministic IPv6 ULA prefix for the lerd network.
// The `1e7d` body is "lerd" in leetspeak, picked to avoid colliding with
// common defaults (fd00::, fd00:beef::, etc.).
const LerdULAv6Subnet = "fd00:1e7d::/64"

// ErrNetworkNeedsMigration is returned by EnsureNetwork when an existing lerd
// network is missing the IPv6 subnet and must be recreated. Callers should
// run MigrateNetworkToIPv6 (or equivalent) and then retry EnsureNetwork.
var ErrNetworkNeedsMigration = errors.New("lerd network exists without IPv6, migration required")

// NetworkGateway returns the gateway IP of the named Podman network.
// Falls back to "127.0.0.1" if it cannot be determined. When the network has
// both v4 and v6 subnets, returns the v4 gateway (which most callers expect
// for backwards compatibility).
func NetworkGateway(name string) string {
	out, err := exec.Command(PodmanBin(), "network", "inspect", name,
		"--format", "{{range .Subnets}}{{if (.Gateway).To4}}{{.Gateway}}{{end}}{{end}}").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		// Fallback for older podman that doesn't expose .To4 in the template.
		out, err = exec.Command(PodmanBin(), "network", "inspect", name,
			"--format", "{{range .Subnets}}{{.Gateway}} {{end}}").Output()
		if err != nil {
			return "127.0.0.1"
		}
		for _, gw := range strings.Fields(string(out)) {
			if !strings.Contains(gw, ":") {
				return gw
			}
		}
		return "127.0.0.1"
	}
	return strings.TrimSpace(string(out))
}

// NetworkHasIPv6 reports whether the named podman network has at least one
// IPv6 subnet configured.
func NetworkHasIPv6(name string) bool {
	out, err := exec.Command(PodmanBin(), "network", "inspect", name,
		"--format", "{{range .Subnets}}{{.Subnet}} {{end}}").Output()
	if err != nil {
		return false
	}
	for _, subnet := range strings.Fields(string(out)) {
		if strings.Contains(subnet, ":") {
			return true
		}
	}
	return false
}

// EnsureNetwork creates the named podman network dual-stack if it doesn't
// exist. Returns ErrNetworkNeedsMigration if it exists v4-only; the caller
// should run MigrateNetworkToIPv6 and retry.
func EnsureNetwork(name string) error {
	out, err := Run("network", "ls", "--format={{.Name}}")
	if err != nil {
		return err
	}

	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			if NetworkHasIPv6(name) {
				return nil
			}
			return ErrNetworkNeedsMigration
		}
	}

	return RunSilent("network", "create",
		"--driver", "bridge",
		"--ipv6",
		"--subnet", LerdULAv6Subnet,
		name)
}

// MigrateNetworkToIPv6 stops and removes containers attached to the named
// network, removes the network, and recreates it dual-stack with v4+v6.
// Returns the removed container names so the caller can recreate them via
// StartUnit; `podman start` would reuse the stale pre-migration network spec.
func MigrateNetworkToIPv6(name string) ([]string, error) {
	dnsOut, err := Run("network", "inspect", name,
		"--format", "{{range .NetworkDNSServers}}{{.}} {{end}}")
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", name, err)
	}
	prevDNS := strings.Fields(strings.TrimSpace(dnsOut))

	containersOut, err := Run("ps", "-a",
		"--filter", "network="+name,
		"--format", "{{.Names}}")
	if err != nil {
		return nil, fmt.Errorf("listing containers on %s: %w", name, err)
	}
	var attached []string
	for _, c := range strings.Split(containersOut, "\n") {
		if c = strings.TrimSpace(c); c != "" {
			attached = append(attached, c)
		}
	}

	for _, c := range attached {
		_ = RunSilent("stop", "--time", "10", c)
		_ = RunSilent("rm", "--force", c)
	}

	if err := RunSilent("network", "rm", "--force", name); err != nil {
		return attached, fmt.Errorf("removing %s: %w", name, err)
	}

	if err := RunSilent("network", "create",
		"--driver", "bridge",
		"--ipv6",
		"--subnet", LerdULAv6Subnet,
		name); err != nil {
		return attached, fmt.Errorf("recreating %s: %w", name, err)
	}

	for _, dns := range prevDNS {
		_ = RunSilent("network", "update", "--dns-add", dns, name)
	}

	return attached, nil
}

// EnsureNetworkDNS syncs the DNS servers on the named network to the provided list.
// It drops servers no longer present and adds new ones. This sets the upstream
// forwarders that aardvark-dns uses, which is necessary on systems where
// /etc/resolv.conf points to a stub resolver (e.g. 127.0.0.53) that is not
// reachable from inside the container network namespace.
func EnsureNetworkDNS(name string, servers []string) error {
	if len(servers) == 0 {
		return nil
	}

	// Get current DNS servers on the network.
	out, err := Run("network", "inspect", name, "--format", "{{range .NetworkDNSServers}}{{.}} {{end}}")
	if err != nil {
		return err
	}

	current := map[string]bool{}
	for _, s := range strings.Fields(out) {
		current[s] = true
	}

	desired := map[string]bool{}
	for _, s := range servers {
		desired[s] = true
	}

	// Drop servers that are no longer desired.
	for s := range current {
		if !desired[s] {
			if err := RunSilent("network", "update", "--dns-drop", s, name); err != nil {
				return err
			}
		}
	}

	// Add servers that are not yet present.
	for s := range desired {
		if !current[s] {
			if err := RunSilent("network", "update", "--dns-add", s, name); err != nil {
				return err
			}
		}
	}

	return nil
}
