//go:build linux

package dns

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

const nmDnsConf = `[main]
dns=dnsmasq
`

const nmDnsmasqConf = `server=/test/127.0.0.1#5300
`

// lerd0 is an always-up dummy interface that keeps .test resolving when every
// real link is down. systemd-resolved refuses to resolve anything (returns
// "Network is down" to NSS and resolvectl alike) once no link is routable, and
// it will not consult a global-scope loopback server in that state. A dummy link
// is always up, so carrying the ~test route on it keeps resolved willing to
// forward .test to lerd-dns on 127.0.0.1:5300 with no network connection at all.
//
// lerd owns the link through a system unit and tells NetworkManager to leave it
// alone. An NM-managed connection would show up as a togglable network in the
// desktop's network menu, where switching it off silently breaks offline .test
// with no symptom until the user next loses the network. Unmanaged also means NM
// never re-pushes DNS over the link, so the resolvectl route it carries stays put.
const lerdNMUnmanaged = "/etc/NetworkManager/conf.d/lerd-dns-link.conf"

// Pre-1.30 builds shipped lerd0 as an NM keyfile connection. Kept so setup can
// migrate those hosts off it and Teardown can clean it up.
const (
	lerdDummyConn    = "lerd-dns"
	lerdDummyKeyfile = "/etc/NetworkManager/system-connections/lerd-dns.nmconnection"
)

// lerdNMUnmanagedContent keeps NetworkManager's hands off lerd0: no entry in the
// desktop network menu, and no DNS re-push over the link.
const lerdNMUnmanagedContent = `[keyfile]
unmanaged-devices=interface-name:lerd0
`

// lerdFallbackDropin turns off systemd-resolved's fallback DNS servers, and is
// the price of lerd0.
//
// Offline, resolved normally answers everything with "Network is down" instantly.
// lerd0 is what stops it doing that, which is the whole point for .test, but the
// same switch also makes resolved willing to chase names it cannot reach: it then
// works through its fallback servers (quad9, Cloudflare, Google) one by one, none
// of which answer with no network, so every offline lookup of a non-.test name
// hangs for 20s or more instead of failing at once. That is not something lerd0
// can dodge; the willingness to serve .test and the willingness to try the
// internet are one and the same flag inside resolved.
//
// Written unconditionally rather than gated on a distro: Debian, Ubuntu and
// Fedora already ship FallbackDNS empty, which is exactly why they never showed
// this, so there it changes nothing. It only bites where the fallbacks are on
// (Arch and its derivatives), and there it aligns them with what the other
// distros already do. The cost is that a broken upstream DNS now fails instead of
// silently routing your queries to a public resolver.
const lerdFallbackDropin = "/etc/systemd/resolved.conf.d/lerd-fallback.conf"

const lerdFallbackDropinContent = `[Resolve]
FallbackDNS=
`

// lerdDummyAddr is lerd0's address. systemd-resolved only gives a link a DNS
// scope once it carries a routable address: with a link-local address alone the
// link reports "Current Scopes: none" and .test does not resolve offline, which
// is the whole point of the link. It is taken from RFC 5737 TEST-NET-1, reserved
// for documentation and required never to appear on a real network, so the /32
// local route it installs cannot shadow a host the user actually needs to reach.
const lerdDummyAddr = "192.0.2.1/32"

// lerdLinkUnitContentFor renders the unit that creates lerd0 and puts the .tld
// route on it. Ordering after systemd-resolved means the resolvectl calls land on
// a resolved that is ready to keep them. Commands run through /bin/sh so PATH
// resolves ip and resolvectl wherever the distro puts them.
func lerdLinkUnitContentFor(tld string) string {
	return fmt.Sprintf(`[Unit]
Description=lerd .%[1]s DNS link
After=systemd-resolved.service
Wants=systemd-resolved.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'ip link show %[2]s >/dev/null 2>&1 || ip link add %[2]s type dummy; ip addr replace %[3]s dev %[2]s; ip link set %[2]s up; resolvectl dns %[2]s 127.0.0.1:5300; resolvectl domain %[2]s ~%[1]s'
ExecStop=/bin/sh -c 'ip link del %[2]s 2>/dev/null || true'

[Install]
WantedBy=multi-user.target
`, tld, lerdDummyIface, lerdDummyAddr)
}

// nmDispatcherScript is installed at /etc/NetworkManager/dispatcher.d/99-lerd-dns.
// On systems with NetworkManager + systemd-resolved, NM manages resolved via DBus and
// overrides global resolved.conf drop-ins. Per-interface DNS set via resolvectl is
// respected. We set two routing domains: ~test routes .test queries to lerd's dnsmasq,
// and ~. keeps the interface as the default route so all other DNS (internet) still works.
// The DHCP-assigned DNS servers are preserved alongside lerd's so internet continues
// to work even when lerd-dns is not yet running.
// When the network changes (LAN↔WiFi, switching networks), the script also rewrites
// the lerd dnsmasq config and restarts lerd-dns so the new upstream DNS is picked up
// immediately without requiring a manual lerd restart.
const nmDispatcherScript = `#!/bin/sh
# Lerd DNS: route .test queries through local dnsmasq on port 5300
IFACE="$1"
ACTION="$2"
LERD_DNS=""

# lerd0 is unmanaged, so NM never dispatches for it and never re-pushes DNS over
# it. Its route is set by lerd-dns-link.service and stays put; nothing to do here.
if [ "$IFACE" = "lerd0" ]; then
    exit 0
fi

if [ "$ACTION" = "up" ] || [ "$ACTION" = "dhcp4-change" ] || [ "$ACTION" = "dhcp6-change" ]; then
    LERD_DNS=$(nmcli -g IP4.DNS device show "$IFACE" 2>/dev/null | tr '|' '\n' | grep -v '^$' | tr '\n' ' ')
    resolvectl dns "$IFACE" 127.0.0.1:5300 $LERD_DNS 2>/dev/null || true
    resolvectl domain "$IFACE" ~test ~. 2>/dev/null || true
elif [ "$ACTION" = "down" ]; then
    # Interface went down: switch lerd-dns to the remaining default interface's DNS
    # so upstream resolution keeps working (e.g. closing wired while on WiFi).
    DEFAULT_IFACE=$(ip route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="dev"){print $(i+1);exit}}')
    [ -n "$DEFAULT_IFACE" ] && [ "$DEFAULT_IFACE" != "$IFACE" ] || exit 0
    LERD_DNS=$(nmcli -g IP4.DNS device show "$DEFAULT_IFACE" 2>/dev/null | tr '|' '\n' | grep -v '^$' | tr '\n' ' ')
else
    exit 0
fi

# Sync lerd-dns config and restart for any user running it. The dispatcher runs
# as root, so the config rewrite is piped through runuser ($as_user) and written
# by the owning user, never by root: a user who symlinks their lerd.conf at a
# root-owned path then can only write where they already could, closing the
# arbitrary-file-write escalation. systemctl --user likewise runs via runuser.
for uid_dir in /run/user/[0-9]*/; do
    [ -d "$uid_dir" ] || continue
    bus="${uid_dir}bus"
    [ -S "$bus" ] || continue
    uid=$(basename "$uid_dir")
    user=$(getent passwd "$uid" | cut -d: -f1)
    home=$(getent passwd "$uid" | cut -d: -f6)
    [ -n "$user" ] && [ -n "$home" ] || continue
    as_user="runuser -u $user -- env XDG_RUNTIME_DIR=$uid_dir DBUS_SESSION_BUS_ADDRESS=unix:path=$bus"
    $as_user systemctl --user is-active lerd-dns >/dev/null 2>&1 || continue
    config_file="$home/.local/share/lerd/dnsmasq/lerd.conf"
    config_yaml="$home/.config/lerd/config.yaml"
    [ -f "$config_file" ] || continue
    tld=$(grep 'tld:' "$config_yaml" 2>/dev/null | sed 's/.*tld:[[:space:]]*//' | sed 's/[^a-zA-Z0-9._-]//g' | head -1)
    tld=${tld:-test}
    # Prefer the user's pinned dns.upstream over the DHCP-detected servers.
    upstream=$(awk '
        /^[^[:space:]#]/ { indns = ($1 == "dns:"); inup = 0 }
        indns && $1 == "upstream:" { inup = 1; next }
        inup && /^[[:space:]]*#/ { next }
        inup && /^[[:space:]]*-/ { sub(/^[[:space:]]*-[[:space:]]*/, ""); sub(/[[:space:]]+#.*/, ""); gsub(/["'\'']/, ""); if ($0 != "") print; next }
        inup && /^[[:space:]]*[^[:space:]-]/ { inup = 0 }
    ' "$config_yaml" 2>/dev/null | tr '\n' ' ')
    dns_servers="$LERD_DNS"
    [ -n "$upstream" ] && dns_servers="$upstream"
    [ -n "$dns_servers" ] || continue
    # Carry lerd's existing address records over untouched. They are lerd policy,
    # loopback normally and the host's LAN IP under lan:expose, and only the Go
    # side knows which applies; regenerating them here clobbered lan:expose back to
    # loopback and dropped the AAAA record, which cost ~20s on every offline .test
    # lookup once the upstream went away. Read before the pipeline: tee truncates
    # config_file as soon as it opens it.
    addr_records=$(grep '^address=/' "$config_file" 2>/dev/null)
    [ -n "$addr_records" ] || addr_records=$(printf 'address=/.%s/127.0.0.1\naddress=/.%s/::1' "$tld" "$tld")
    {
        printf '# Lerd DNS configuration\nport=5300\nno-resolv\n'
        for dns_ip in $dns_servers; do
            # Defensive filter: emit only tokens shaped like an IP with an
            # optional #port. The Go side validates dns.upstream, but the awk
            # re-parse above does not, so reject anything outside IP/port chars.
            case "$dns_ip" in
                ''|*[!0-9A-Fa-f:.#]*) continue ;;
            esac
            printf 'server=%s\n' "$dns_ip"
        done
        printf '%s\n' "$addr_records"
    } | $as_user tee "$config_file" >/dev/null
    $as_user systemctl --user restart lerd-dns 2>/dev/null || true
done
`

// isSystemdResolvedActive returns true if systemd-resolved is the active DNS resolver.
var isSystemdResolvedActive = func() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved")
	if err := cmd.Run(); err != nil {
		return false
	}
	// Also check that /etc/resolv.conf points to the stub resolver
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "127.0.0.53") || strings.Contains(string(data), "systemd-resolved")
}

// isNetworkManagerActive returns true if NetworkManager is running.
var isNetworkManagerActive = func() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "NetworkManager")
	return cmd.Run() == nil
}

// ResolverHint returns a user-facing hint for restarting the active DNS resolver.
func ResolverHint() string {
	if isNetworkManagerActive() {
		return "sudo systemctl restart NetworkManager"
	}
	if isSystemdResolvedActive() {
		return "sudo systemctl restart systemd-resolved"
	}
	return "restart your DNS resolver"
}

// lerdDNSInterfaces returns all network interfaces that currently have
// 127.0.0.1:5300 configured as a DNS server (set by the lerd dispatcher).
func lerdDNSInterfaces() []string {
	out, err := exec.Command("resolvectl", "status").Output()
	if err != nil {
		// Fallback to just the default interface.
		if iface := defaultInterface(); iface != "" {
			return []string{iface}
		}
		return nil
	}
	return parseLerdDNSInterfaces(string(out))
}

// parseLerdDNSInterfaces extracts interface names from resolvectl status output
// that have 127.0.0.1:5300 configured as a DNS server.
func parseLerdDNSInterfaces(output string) []string {
	var ifaces []string
	var currentIface string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Link ") {
			if start := strings.Index(line, "("); start >= 0 {
				if end := strings.Index(line, ")"); end > start {
					currentIface = line[start+1 : end]
				}
			}
		}
		if currentIface != "" && strings.Contains(line, "127.0.0.1:5300") {
			ifaces = append(ifaces, currentIface)
			currentIface = ""
		}
	}
	return ifaces
}

// defaultInterface returns the name of the default network interface (e.g. "enp1s0").
func defaultInterface() string {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}
	return parseDefaultIface(string(out))
}

// parseDefaultIface extracts the interface name from `ip route show default` output.
func parseDefaultIface(output string) string {
	// "default via 192.168.1.1 dev enp1s0 ..."
	parts := strings.Fields(output)
	for i, p := range parts {
		if p == "dev" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// resolvPaths is the ordered list of resolv.conf files to try for upstream DNS detection.
// Overridable in tests.
var resolvPaths = []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"}

// nmcliDNSFunc is the function used to get DHCP DNS via nmcli. Overridable in tests.
var nmcliDNSFunc = func() []string {
	out, err := exec.Command("nmcli", "-g", "IP4.DNS", "device", "show").Output()
	if err != nil {
		return nil
	}
	return parseNmcliLines(string(out))
}

// defaultUpstreamFallback returns the last-resort dnsmasq upstream when no
// system-detected nameservers are usable. On Linux, pasta's 169.254.1.1
// bridges into the host resolver and preserves .test routing.
func defaultUpstreamFallback() []string {
	return []string{pastaDefaultForwarder}
}

// ReadContainerDNS returns DNS servers for aardvark-dns on the lerd network,
// preferring pasta's info.json (typically 169.254.1.1) and falling back to
// host upstreams then pastaDefaultForwarder so the list is never empty.
func ReadContainerDNS() []string {
	path := fmt.Sprintf("/run/user/%d/containers/networks/rootless-netns/info.json", os.Getuid())
	data, err := os.ReadFile(path)
	if err != nil {
		return upstreamOrPasta()
	}
	var info struct {
		DnsForwardIps []string `json:"DnsForwardIps"`
	}
	if err := json.Unmarshal(data, &info); err != nil || len(info.DnsForwardIps) == 0 {
		return upstreamOrPasta()
	}
	var out []string
	for _, ip := range info.DnsForwardIps {
		if clean := sanitizeDNSIP(ip); clean != "" {
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		return upstreamOrPasta()
	}
	return out
}

// upstreamOrPasta returns host upstreams when readable, else pasta's default
// forwarder, so the lerd network never ends up with an empty DNS list.
func upstreamOrPasta() []string {
	if servers := readUpstreamDNS(); len(servers) > 0 {
		return servers
	}
	return []string{pastaDefaultForwarder}
}

// ReadUpstreamDNS returns upstream DNS server IPs from the running system.
// Sources tried in order:
//  1. /run/systemd/resolve/resolv.conf — real upstreams on systemd-resolved systems
//  2. /etc/resolv.conf — fallback
//  3. nmcli — DHCP-provided DNS from NetworkManager
//
// Returns nil if nothing is found; callers should omit no-resolv in that case.
func ReadUpstreamDNS() []string {
	return readUpstreamDNS()
}

// readUpstreamDNS is the internal implementation.
func readUpstreamDNS() []string {
	if servers := configuredUpstreamDNS(); len(servers) > 0 {
		return servers
	}
	for _, path := range resolvPaths {
		if servers := parseNameservers(path); len(servers) > 0 {
			return servers
		}
	}
	return nmcliDNSFunc()
}

// nmcliDNS reads DHCP-assigned DNS servers from NetworkManager via nmcli.
func nmcliDNS() []string {
	return nmcliDNSFunc()
}

// parseNmcliLines parses the output of `nmcli -g IP4.DNS device show`.
func parseNmcliLines(output string) []string {
	var servers []string
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		// nmcli may separate multiple values with |
		for _, ip := range strings.Split(line, "|") {
			clean := sanitizeDNSIP(ip)
			if clean == "" {
				continue
			}
			if !seen[clean] {
				seen[clean] = true
				servers = append(servers, clean)
			}
		}
	}
	return servers
}

// Setup writes DNS configuration for .test resolution and restarts the resolver.
// On systemd-resolved + NetworkManager systems (Ubuntu etc.) it uses an NM dispatcher script.
// On pure systemd-resolved systems it uses a resolved drop-in.
// On NetworkManager-only systems it uses NM's embedded dnsmasq.
//
// Deprecated: prefer calling WriteDnsmasqConfig then ConfigureResolver separately so
// that the dnsmasq container can be started between the two steps.
func Setup() error {
	if err := WriteDnsmasqConfig(config.DnsmasqDir()); err != nil {
		return fmt.Errorf("writing lerd dnsmasq config: %w", err)
	}
	return ConfigureResolver()
}

// ConfigureResolver configures the system DNS resolver to forward .test to the
// lerd-dns dnsmasq container on port 5300. Call this after lerd-dns is running so
// that any immediate resolvectl changes don't break DNS before dnsmasq is up.
func ConfigureResolver() error {
	if isSystemdResolvedActive() {
		if isNetworkManagerActive() {
			return setupNMWithResolved()
		}
		return setupSystemdResolved()
	}
	return setupNetworkManager()
}

// dummyLinkHealthy reports whether lerd0 exists and still carries the .test route.
// resolved keeps per-link config across its own restarts (it stashes it under
// /run/systemd/resolve/netif) but not across a reboot, and nothing stops a user
// deleting the link by hand, so this is checked on every start rather than assumed.
var dummyLinkHealthy = func(tld string) bool {
	present, routed := defaultDummyLinkRouting(tld)
	return present && routed
}

// dummyLinkUnitEnabled reports whether the link unit is wired to start at boot.
// Separate from dummyLinkHealthy: the link being up right now says nothing about
// whether it will come back after a reboot. No sudo needed to ask.
var dummyLinkUnitEnabled = func() bool {
	return exec.Command("systemctl", "is-enabled", "--quiet", lerdLinkUnitName).Run() == nil
}

// dummyLinkGrantsLive reports whether the drop-in grants what setupDummyLink
// needs, passwordless, without prompting for anything.
//
// It runs a granted command rather than asking `sudo -l` whether the user may run
// one. `sudo -n -l <cmd>` answers "may george run this at all", which is yes for
// any user in the sudo/wheel group even with no lerd grants whatsoever, so it can
// never fail on a normal desktop. Running `mkdir -p` on a directory that already
// exists is granted, idempotent, and answers the question that matters: does this
// go through without a password.
var dummyLinkGrantsLive = func() bool {
	return exec.Command("sudo", "-n", "mkdir", "-p", "/etc/systemd/system").Run() == nil
}

// dummyLinkNMRuleNeeded reports whether the NetworkManager unmanaged rule has to
// be written. Without NetworkManager running there is nothing to keep off the
// link, and writing it would create an /etc/NetworkManager tree on a host that
// has no NetworkManager installed.
func dummyLinkNMRuleNeeded(withNM bool) bool {
	return withNM && !isFileContent(lerdNMUnmanaged, []byte(lerdNMUnmanagedContent))
}

// setupDummyLink provisions lerd0: a dummy link carrying the ~test route so
// .test still resolves when every real interface is down. Both resolved paths
// need it: resolved refuses a loopback DNS server once no link is routable
// whether that server is per-link or global, so the NetworkManager-less case
// (Arch without NM, omarchy) fails offline exactly like the NM one.
//
// withNM keeps NetworkManager out of the link's way. It is false when NM isn't
// running, where writing the rule would mean creating an /etc/NetworkManager
// tree on a host that has no NetworkManager at all.
func setupDummyLink(withNM bool, tld string) error {
	if !dummyLinkGrantsLive() {
		return fmt.Errorf("sudoers drop-in is out of date, run `lerd install` to refresh it")
	}
	// Migrate hosts that got lerd0 as an NM keyfile connection from a pre-release
	// build. Deleting the connection drops the link; the unit below recreates it.
	if _, err := os.Stat(lerdDummyKeyfile); err == nil {
		exec.Command("sudo", "nmcli", "connection", "delete", lerdDummyConn).Run() //nolint:errcheck
		exec.Command("sudo", "rm", "-f", lerdDummyKeyfile).Run()                   //nolint:errcheck
	}

	unitContent := lerdLinkUnitContentFor(tld)
	unitChanged := !isFileContent(lerdLinkUnit, []byte(unitContent))
	nmChanged := dummyLinkNMRuleNeeded(withNM)
	fallbackChanged := !isFileContent(lerdFallbackDropin, []byte(lerdFallbackDropinContent))
	if unitChanged || nmChanged || fallbackChanged {
		feedback.Sudo("Configuring an always-up link so ." + tld + " resolves offline")
	}
	if nmChanged {
		if err := sudoWriteFile(lerdNMUnmanaged, []byte(lerdNMUnmanagedContent), 0644); err != nil {
			return fmt.Errorf("writing NetworkManager unmanaged rule: %w", err)
		}
		exec.Command("sudo", "systemctl", "reload", "NetworkManager").Run() //nolint:errcheck
	}
	if unitChanged {
		if err := sudoWriteFile(lerdLinkUnit, []byte(unitContent), 0644); err != nil {
			return fmt.Errorf("writing lerd-dns-link unit: %w", err)
		}
		exec.Command("sudo", "systemctl", "daemon-reload").Run() //nolint:errcheck
	}

	// Before the link is brought up, not after: this restarts resolved, and the
	// route lerd0 carries is applied by the unit below. Doing it the other way
	// round would lean on resolved restoring per-link config across a restart.
	if fallbackChanged {
		if err := sudoWriteFile(lerdFallbackDropin, []byte(lerdFallbackDropinContent), 0644); err != nil {
			return fmt.Errorf("writing resolved fallback drop-in: %w", err)
		}
		exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run() //nolint:errcheck
	}

	// Outside the changed-check above: on every start after the first the files are
	// identical, and that is exactly when the link may be missing (fresh boot, or
	// someone removed it). Gating on a config change would leave lerd0 down with no
	// way back short of a reboot.
	return ensureDummyLinkRunning(tld)
}

// ensureDummyLinkRunning enables the link unit for the next boot and starts it if
// lerd0 isn't currently carrying the route. It reports whether lerd0 ended up
// actually carrying it: callers rely on the link before removing the older
// hookups it replaces, so a silent failure here would strand a host with neither.
//
// Enablement is checked separately from health rather than folded into one
// `enable --now`: enabled is what brings lerd0 back after a reboot, so a link
// that merely happens to be up right now must not stop us enabling the unit, or
// it survives until the next boot and then silently disappears.
func ensureDummyLinkRunning(tld string) error {
	if !dummyLinkUnitEnabled() {
		exec.Command("sudo", "systemctl", "enable", "--now", lerdLinkUnitName).Run() //nolint:errcheck
	}
	if !dummyLinkHealthy(tld) {
		exec.Command("sudo", "systemctl", "restart", lerdLinkUnitName).Run() //nolint:errcheck
	}
	if !dummyLinkHealthy(tld) {
		return fmt.Errorf("%s is not carrying the ~%s route (check: systemctl status %s)",
			lerdDummyIface, tld, lerdLinkUnitName)
	}
	return nil
}

// setupNMWithResolved handles Ubuntu-style: NM manages systemd-resolved via DBUS.
// NM overrides per-interface DNS, so an NM dispatcher script applies the interface
// route via resolvectl on each "up" event and immediately to the current default
// interface. That per-link route dies with the interface, so an always-up unmanaged
// dummy link (lerd0) carries the ~test route to keep .test resolving offline.
func setupNMWithResolved() error {
	dispatcherScript := "/etc/NetworkManager/dispatcher.d/99-lerd-dns"

	if !isFileContent(dispatcherScript, []byte(nmDispatcherScript)) {
		feedback.Sudo("Configuring NetworkManager dispatcher for .test DNS resolution")

		if err := sudoWriteFile(dispatcherScript, []byte(nmDispatcherScript), 0755); err != nil {
			return fmt.Errorf("writing NM dispatcher script: %w", err)
		}
	}

	// Remove a stale resolved drop-in from an install that predates the dispatcher.
	// It doesn't work under NM, which overrides global DNS, and leaving it behind
	// makes `lerd dns:diagnose` report the wrong resolver hookup.
	dropin := "/etc/systemd/resolved.conf.d/lerd.conf"
	if _, err := os.Stat(dropin); err == nil {
		rmCmd := exec.Command("sudo", "rm", "-f", dropin)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run() //nolint:errcheck
	}

	if err := setupDummyLink(true, ConfiguredTLD()); err != nil {
		return err
	}

	// Apply immediately to the current default interface.
	// Include DHCP-assigned upstream DNS servers alongside lerd's so internet
	// continues to work even when lerd-dns is not running.
	iface := defaultInterface()
	if iface == "" {
		return nil
	}

	// Revert the interface to clear any stale DNS server failure state from boot.
	// At boot, the NM dispatcher sets 127.0.0.1:5300 before lerd-dns starts; resolved
	// marks it failed and promotes the fallback to "current". Calling resolvectl with
	// the same list later does not reset the current server. Reverting first forces a
	// clean slate so our subsequent dns call starts with 127.0.0.1:5300 as current.
	revertCmd := exec.Command("sudo", "resolvectl", "revert", iface)
	revertCmd.Stdin = os.Stdin
	revertCmd.Stdout = os.Stdout
	revertCmd.Stderr = os.Stderr
	revertCmd.Run() //nolint:errcheck

	dnsArgs := []string{"sudo", "resolvectl", "dns", iface, "127.0.0.1:5300"}
	dnsArgs = append(dnsArgs, readUpstreamDNS()...)
	dnsCmd := exec.Command(dnsArgs[0], dnsArgs[1:]...)
	dnsCmd.Stdin = os.Stdin
	dnsCmd.Stdout = os.Stdout
	dnsCmd.Stderr = os.Stderr
	if err := dnsCmd.Run(); err != nil {
		return fmt.Errorf("applying DNS to %s: %w", iface, err)
	}

	domainCmd := exec.Command("sudo", "resolvectl", "domain", iface, "~test", "~.")
	domainCmd.Stdin = os.Stdin
	domainCmd.Stdout = os.Stdout
	domainCmd.Stderr = os.Stderr
	if err := domainCmd.Run(); err != nil {
		return fmt.Errorf("applying domain routing to %s: %w", iface, err)
	}

	// Keep dnsmasq config in sync with the upstream DNS servers now active on
	// the interface. resolvectl has just updated systemd-resolved, so
	// readUpstreamDNS() will return the current (post-change) upstreams.
	// Restart lerd-dns only when the config actually changes to avoid
	// unnecessary downtime on normal starts where nothing has changed.
	existing, _ := os.ReadFile(filepath.Join(config.DnsmasqDir(), "lerd.conf"))
	if err := WriteDnsmasqConfig(config.DnsmasqDir()); err == nil {
		updated, _ := os.ReadFile(filepath.Join(config.DnsmasqDir(), "lerd.conf"))
		if string(existing) != string(updated) {
			exec.Command("systemctl", "--user", "restart", "lerd-dns").Run() //nolint:errcheck
		}
	}

	return nil
}

// setupSystemdResolved points .test at lerd-dns when systemd-resolved runs
// without NetworkManager (Arch, omarchy).
//
// lerd0 is the whole mechanism here. lerd used to declare the resolver globally
// instead, in /etc/systemd/resolved.conf.d/lerd.conf, but that drop-in was both
// insufficient and harmful. Insufficient because resolved refuses a global
// loopback server once no link is routable, exactly as it refuses a per-link one,
// so .test died offline anyway. Harmful because a global DNS server is a
// catch-all: every ordinary name went to lerd-dns too, and offline dnsmasq
// forwarded it to an upstream that wasn't there, so each lookup hung ~20s instead
// of failing at once. lerd0 carries the same route scoped to ~test only, which
// fixes both, so the drop-in is removed rather than written.
func setupSystemdResolved() error {
	tld := ConfiguredTLD()

	// Provision the link first, and only drop the legacy global drop-in once lerd0
	// demonstrably carries the route. Removing first and creating second leaves a
	// host with neither whenever provisioning fails, and the drop-in it supersedes
	// at least resolves .tld while a link is up.
	if err := setupDummyLink(false, tld); err != nil {
		return err
	}

	dropin := "/etc/systemd/resolved.conf.d/lerd.conf"
	if _, err := os.Stat(dropin); err != nil {
		return nil
	}
	feedback.Sudo("Removing the superseded systemd-resolved drop-in")
	if err := exec.Command("sudo", "rm", "-f", dropin).Run(); err != nil {
		return fmt.Errorf("removing superseded resolved drop-in: %w", err)
	}
	exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run() //nolint:errcheck
	// That restart drops per-link config, so put the route back on lerd0.
	return ensureDummyLinkRunning(tld)
}

// setupNetworkManager configures NetworkManager's embedded dnsmasq.
func setupNetworkManager() error {
	nmConfFile := "/etc/NetworkManager/conf.d/lerd.conf"
	nmDnsmasqFile := "/etc/NetworkManager/dnsmasq.d/lerd.conf"

	if isFileContent(nmConfFile, []byte(nmDnsConf)) && isFileContent(nmDnsmasqFile, []byte(nmDnsmasqConf)) {
		return nil
	}

	feedback.Sudo("Configuring NetworkManager for .test DNS resolution")

	if err := sudoWriteFile(nmConfFile, []byte(nmDnsConf), 0644); err != nil {
		return fmt.Errorf("writing NetworkManager conf: %w", err)
	}

	if err := sudoWriteFile(nmDnsmasqFile, []byte(nmDnsmasqConf), 0644); err != nil {
		return fmt.Errorf("writing NetworkManager dnsmasq conf: %w", err)
	}

	cmd := exec.Command("sudo", "systemctl", "restart", "NetworkManager")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restarting NetworkManager: %w", err)
	}
	return nil
}

// Teardown removes all lerd DNS configuration from the system and restores normal resolution.
func Teardown() {
	// NM dispatcher script
	dispatcherScript := "/etc/NetworkManager/dispatcher.d/99-lerd-dns"
	if _, err := os.Stat(dispatcherScript); err == nil {
		rmCmd := exec.Command("sudo", "rm", "-f", dispatcherScript)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run() //nolint:errcheck
	}

	// The global resolved drop-in. lerd stopped writing it once lerd0 took over the
	// .test route, so this only finds it on installs that predate the link.
	dropin := "/etc/systemd/resolved.conf.d/lerd.conf"
	if _, err := os.Stat(dropin); err == nil {
		rmCmd := exec.Command("sudo", "rm", "-f", dropin)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run() //nolint:errcheck
	}

	// The always-up dummy link. Disabling the unit runs its ExecStop, which deletes
	// lerd0; the explicit link delete covers a host where the unit file is already
	// gone but the link is still up.
	if _, err := os.Stat(lerdLinkUnit); err == nil {
		disableCmd := exec.Command("sudo", "systemctl", "disable", "--now", lerdLinkUnitName)
		disableCmd.Stdin = os.Stdin
		disableCmd.Stdout = os.Stdout
		disableCmd.Stderr = os.Stderr
		disableCmd.Run() //nolint:errcheck
	}
	// Guarded: unguarded this prompts for a password to delete an interface that
	// was never there, on every host that used a different resolver path.
	if exec.Command("ip", "link", "show", lerdDummyIface).Run() == nil {
		exec.Command("sudo", "ip", "link", "del", lerdDummyIface).Run() //nolint:errcheck
	}

	// The NM keyfile connection from a pre-release build, if this host ever ran one.
	if _, err := os.Stat(lerdDummyKeyfile); err == nil {
		delCmd := exec.Command("sudo", "nmcli", "connection", "delete", lerdDummyConn)
		delCmd.Stdin = os.Stdin
		delCmd.Stdout = os.Stdout
		delCmd.Stderr = os.Stderr
		delCmd.Run() //nolint:errcheck
		rmCmd := exec.Command("sudo", "rm", "-f", lerdDummyKeyfile)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run() //nolint:errcheck
	}

	// Give the system its fallback DNS servers back: they were only turned off to
	// stop lerd0 making offline lookups hang, and with lerd0 gone that reason goes
	// with it. resolved is restarted below.
	if _, err := os.Stat(lerdFallbackDropin); err == nil {
		rmCmd := exec.Command("sudo", "rm", "-f", lerdFallbackDropin)
		rmCmd.Stdin = os.Stdin
		rmCmd.Stdout = os.Stdout
		rmCmd.Stderr = os.Stderr
		rmCmd.Run()                                                            //nolint:errcheck
		exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run() //nolint:errcheck
	}

	// NetworkManager conf and dnsmasq conf
	for _, f := range []string{
		"/etc/NetworkManager/conf.d/lerd.conf",
		"/etc/NetworkManager/dnsmasq.d/lerd.conf",
		lerdNMUnmanaged,
		lerdLinkUnit,
	} {
		if _, err := os.Stat(f); err == nil {
			rmCmd := exec.Command("sudo", "rm", "-f", f)
			rmCmd.Stdin = os.Stdin
			rmCmd.Stdout = os.Stdout
			rmCmd.Stderr = os.Stderr
			rmCmd.Run() //nolint:errcheck
		}
	}
	exec.Command("sudo", "systemctl", "daemon-reload").Run() //nolint:errcheck

	// Revert ALL interfaces that have lerd DNS (127.0.0.1:5300) configured.
	// The dispatcher script applies DNS to every interface on "up", not just
	// the default one, so reverting only the default leaves virtual bridges
	// (virbr0, vnet*) pointing at the dead dnsmasq port.
	for _, iface := range lerdDNSInterfaces() {
		revertCmd := exec.Command("sudo", "resolvectl", "revert", iface)
		revertCmd.Stdin = os.Stdin
		revertCmd.Stdout = os.Stdout
		revertCmd.Stderr = os.Stderr
		revertCmd.Run() //nolint:errcheck
	}

	// Restart the resolver to apply the removal and re-establish upstream DNS.
	if isNetworkManagerActive() {
		feedback.Line("Restarting NetworkManager (may take a moment)…")
		cmd := exec.Command("sudo", "systemctl", "restart", "NetworkManager")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run() //nolint:errcheck

		// NM restart doesn't always re-push DHCP DNS to resolved after a
		// resolvectl revert. Explicitly apply the DHCP-assigned servers so
		// internet DNS works immediately after uninstall.
		if iface := defaultInterface(); iface != "" {
			upstreams := nmcliDNSFunc()
			if len(upstreams) > 0 {
				args := append([]string{"sudo", "resolvectl", "dns", iface}, upstreams...)
				pushCmd := exec.Command(args[0], args[1:]...)
				pushCmd.Stdin = os.Stdin
				pushCmd.Stdout = os.Stdout
				pushCmd.Stderr = os.Stderr
				pushCmd.Run() //nolint:errcheck
			}
		}
	} else if isSystemdResolvedActive() {
		exec.Command("sudo", "systemctl", "restart", "systemd-resolved").Run() //nolint:errcheck
	}
}

// InstallSudoers writes a sudoers drop-in granting the current user passwordless
// access to resolvectl commands. This is required for the autostart service which
// runs non-interactively and cannot prompt for a sudo password.
func InstallSudoers() error {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	if user == "" {
		return fmt.Errorf("cannot determine current user")
	}

	content := renderLinuxSudoers(user)

	const sudoersPath = "/etc/sudoers.d/lerd"
	if sudoersInstalled([]byte(content)) {
		return nil
	}

	feedback.Sudo("Installing DNS sudoers rule")
	if err := sudoWriteFile(sudoersPath, []byte(content), 0440); err != nil {
		return fmt.Errorf("writing sudoers drop-in: %w", err)
	}
	recordSudoersInstalled([]byte(content))
	return nil
}

// renderLinuxSudoers returns the sudoers drop-in content for the given user.
// Every rule uses a fully qualified command argument so modern strict
// parsers (sudo-rs on Ubuntu 26.04+, C sudo >= 1.9.16 on Fedora 41+ /
// Arch / CachyOS / openSUSE Tumbleweed / NixOS unstable) accept the file.
// The resolvectl line drops the per-verb "*" suffixes that older lerd
// builds shipped — sudoers cannot match a verb plus open-ended args
// without a wildcard, and "any resolvectl invocation" is the same
// effective grant since the watcher already calls every verb.
func renderLinuxSudoers(user string) string {
	return fmt.Sprintf(
		"# Lerd: passwordless DNS resolver / NM dispatcher operations.\n"+
			"# Rules are fully qualified with no wildcards in command\n"+
			"# arguments so they pass strict sudo parsers (sudo-rs on Ubuntu\n"+
			"# 26.04+; C sudo >= 1.9.16 on Fedora 41+, Arch, openSUSE\n"+
			"# Tumbleweed, NixOS unstable). The matching code path pipes\n"+
			"# content through `sudo tee <dest>` instead of\n"+
			"# `sudo cp /tmp/lerd-sudo-* <dest>` for the same reason.\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/resolvectl\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/mkdir -p /etc/NetworkManager/dispatcher.d\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/tee /etc/NetworkManager/dispatcher.d/99-lerd-dns\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/chmod 755 /etc/NetworkManager/dispatcher.d/99-lerd-dns\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/mkdir -p /etc/systemd/resolved.conf.d\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/rm -f /etc/systemd/resolved.conf.d/lerd.conf\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/systemctl restart systemd-resolved\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/tee /etc/systemd/resolved.conf.d/lerd-fallback.conf\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/chmod 644 /etc/systemd/resolved.conf.d/lerd-fallback.conf\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/mkdir -p /etc/systemd/system\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/tee /etc/systemd/system/lerd-dns-link.service\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/chmod 644 /etc/systemd/system/lerd-dns-link.service\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/mkdir -p /etc/NetworkManager/conf.d\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/tee /etc/NetworkManager/conf.d/lerd-dns-link.conf\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/chmod 644 /etc/NetworkManager/conf.d/lerd-dns-link.conf\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/systemctl daemon-reload\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/systemctl enable --now lerd-dns-link.service\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/systemctl restart lerd-dns-link.service\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/systemctl reload NetworkManager\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/nmcli connection delete lerd-dns\n"+
			"%s ALL=(root) NOPASSWD: /usr/bin/rm -f /etc/NetworkManager/system-connections/lerd-dns.nmconnection\n",
		user, user, user, user, user, user, user, user, user, user, user,
		user, user, user, user, user, user, user, user, user, user,
	)
}
