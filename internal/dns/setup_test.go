//go:build linux

package dns

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/geodro/lerd/internal/feedback"
)

// --- parseNmcliOutput ---

func TestParseNmcliOutput_basic(t *testing.T) {
	input := "192.168.1.1\n8.8.8.8\n\n"
	got := parseNmcliLines(input)
	want := []string{"192.168.1.1", "8.8.8.8"}
	assertSliceEqual(t, got, want)
}

func TestParseNmcliOutput_pipeSeparated(t *testing.T) {
	input := "192.168.1.1|8.8.8.8\n"
	got := parseNmcliLines(input)
	want := []string{"192.168.1.1", "8.8.8.8"}
	assertSliceEqual(t, got, want)
}

func TestParseNmcliOutput_skipsLoopbackAndDash(t *testing.T) {
	input := "127.0.0.53\n--\n\n10.0.0.1\n127.0.0.1\n"
	got := parseNmcliLines(input)
	want := []string{"10.0.0.1"}
	assertSliceEqual(t, got, want)
}

func TestParseNmcliOutput_deduplicates(t *testing.T) {
	input := "8.8.8.8\n8.8.8.8\n8.8.4.4\n"
	got := parseNmcliLines(input)
	want := []string{"8.8.8.8", "8.8.4.4"}
	assertSliceEqual(t, got, want)
}

func TestParseNmcliOutput_skipsZonedLinkLocal(t *testing.T) {
	input := "fe80::46d4:53ff:fe3f:a9a7%18|8.8.8.8\nfe80::1%eth0\n"
	got := parseNmcliLines(input)
	want := []string{"8.8.8.8"}
	assertSliceEqual(t, got, want)
}

func TestParseNmcliOutput_empty(t *testing.T) {
	got := parseNmcliLines("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- upstreamOrPasta ---

func TestUpstreamOrPasta_usesUpstreamsWhenPresent(t *testing.T) {
	fakeResolv := writeTempFile(t, "nameserver 8.8.8.8\n")
	origPaths := resolvPaths
	resolvPaths = []string{fakeResolv}
	defer func() { resolvPaths = origPaths }()

	got := upstreamOrPasta()
	assertSliceEqual(t, got, []string{"8.8.8.8"})
}

func TestUpstreamOrPasta_fallsBackToPastaForwarder(t *testing.T) {
	emptyResolv := writeTempFile(t, "# empty\n")
	origPaths := resolvPaths
	origNmcli := nmcliDNSFunc
	resolvPaths = []string{emptyResolv}
	nmcliDNSFunc = func() []string { return nil }
	defer func() { resolvPaths = origPaths; nmcliDNSFunc = origNmcli }()

	got := upstreamOrPasta()
	assertSliceEqual(t, got, []string{pastaDefaultForwarder})
}

// --- parseDefaultInterface ---

func TestParseDefaultInterface_typical(t *testing.T) {
	input := "default via 192.168.1.1 dev enp1s0 proto dhcp src 192.168.1.100 metric 100"
	got := parseDefaultIface(input)
	if got != "enp1s0" {
		t.Errorf("expected enp1s0, got %q", got)
	}
}

func TestParseDefaultInterface_wifi(t *testing.T) {
	input := "default via 10.0.0.1 dev wlp2s0 proto dhcp metric 600"
	got := parseDefaultIface(input)
	if got != "wlp2s0" {
		t.Errorf("expected wlp2s0, got %q", got)
	}
}

func TestParseDefaultInterface_multipleRoutes(t *testing.T) {
	input := "default via 192.168.1.1 dev eth0 proto dhcp\ndefault via 10.0.0.1 dev eth1 proto static"
	got := parseDefaultIface(input)
	if got != "eth0" {
		t.Errorf("expected eth0, got %q", got)
	}
}

func TestParseDefaultInterface_empty(t *testing.T) {
	got := parseDefaultIface("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- WriteDnsmasqConfig ---

func TestWriteDnsmasqConfig_withUpstreams(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 192.168.1.1\nnameserver 8.8.8.8\n")
	origPaths := resolvPaths
	resolvPaths = []string{fakeResolv}
	defer func() { resolvPaths = origPaths }()

	if err := WriteDnsmasqConfig(dir); err != nil {
		t.Fatalf("WriteDnsmasqConfig: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))

	assertContains(t, content, "port=5300")
	assertContains(t, content, "no-resolv")
	assertContains(t, content, "server=192.168.1.1")
	assertContains(t, content, "server=8.8.8.8")
	assertContains(t, content, "address=/.test/127.0.0.1")
}

func TestWriteDnsmasqConfig_noUpstreamsFallsBackToPasta(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 127.0.0.53\n")
	origPaths := resolvPaths
	origNmcli := nmcliDNSFunc
	resolvPaths = []string{fakeResolv}
	nmcliDNSFunc = func() []string { return nil }
	defer func() {
		resolvPaths = origPaths
		nmcliDNSFunc = origNmcli
	}()

	if err := WriteDnsmasqConfig(dir); err != nil {
		t.Fatalf("WriteDnsmasqConfig: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))

	assertContains(t, content, "port=5300")
	assertContains(t, content, "address=/.test/127.0.0.1")
	assertContains(t, content, "address=/.test/::1")
	assertContains(t, content, "no-resolv")
	assertContains(t, content, "server="+pastaDefaultForwarder)
	if strings.Contains(content, "listen-address") {
		t.Errorf("dnsmasq must not restrict listen-address (rootlessport forwards via container netif, not loopback), got:\n%s", content)
	}
}

func TestWriteDnsmasqConfig_pinnedUpstreamOverridesResolv(t *testing.T) {
	writeGlobalConfig(t, "dns:\n  upstream:\n    - 192.168.100.129\n")
	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 9.9.9.9\nnameserver 8.8.8.8\n")
	origPaths := resolvPaths
	resolvPaths = []string{fakeResolv}
	defer func() { resolvPaths = origPaths }()

	if err := WriteDnsmasqConfig(dir); err != nil {
		t.Fatalf("WriteDnsmasqConfig: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))
	assertContains(t, content, "server=192.168.100.129")
	if strings.Contains(content, "server=9.9.9.9") || strings.Contains(content, "server=8.8.8.8") {
		t.Errorf("pinned upstream must replace detected resolv.conf servers, got:\n%s", content)
	}
}

// --- NM dispatcher script ---

func TestNMDispatcherScript_runsAsRealUser(t *testing.T) {
	assertContains(t, nmDispatcherScriptFor("test"), "runuser -u")
}

func TestNMDispatcherScript_prefersPinnedUpstream(t *testing.T) {
	assertContains(t, nmDispatcherScriptFor("test"), "upstream:")
	assertContains(t, nmDispatcherScriptFor("test"), "dns_servers=\"$LERD_DNS\"")
}

// The dispatcher runs as root, so writing the per-user lerd.conf with a plain
// root `> "$config_file"` redirect lets a user symlink that path at a root-owned
// file and have root truncate it (CWE-59 privesc). The write must go through
// runuser ($as_user) so it happens with the owning user's privileges.
func TestNMDispatcherScript_writesConfigAsUser(t *testing.T) {
	assertContains(t, nmDispatcherScriptFor("test"), `| $as_user tee "$config_file"`)
	if strings.Contains(nmDispatcherScriptFor("test"), `} > "$config_file"`) {
		t.Error("dispatcher still writes lerd.conf via a root redirect; must pipe through $as_user")
	}
}

// The awk re-parse of dns.upstream applies no validation, so the server-entry
// loop must filter to IP/port-shaped tokens before emitting server= lines.
func TestNMDispatcherScript_filtersUpstreamEntries(t *testing.T) {
	assertContains(t, nmDispatcherScriptFor("test"), "*[!0-9A-Fa-f:.#]*) continue")
}

// The address records are lerd policy: loopback normally, the host's LAN IP
// under lan:expose, and only the Go side knows which. The dispatcher used to
// regenerate them from a hardcoded template on every interface "up", which
// clobbered lan:expose back to loopback and dropped the AAAA record (costing
// ~20s per offline .test lookup). It must carry the existing records over.
func TestNMDispatcherScript_preservesAddressRecords(t *testing.T) {
	assertContains(t, nmDispatcherScriptFor("test"), `addr_records=$(grep '^address=/' "$config_file"`)
	assertContains(t, nmDispatcherScriptFor("test"), `printf '%s\n' "$addr_records"`)
	if strings.Contains(nmDispatcherScriptFor("test"), `printf 'address=/.%s/127.0.0.1\n' "$tld"`) {
		t.Error("dispatcher must not regenerate address records; it clobbers lan:expose and drops the AAAA record")
	}
}

// The address records must be read before the rewrite pipeline runs: tee
// truncates config_file the moment it opens it, so a grep inside the pipeline
// races against an already-empty file and would silently drop the records.
func TestNMDispatcherScript_readsAddressRecordsBeforePipeline(t *testing.T) {
	grepAt := strings.Index(nmDispatcherScriptFor("test"), `addr_records=$(grep '^address=/'`)
	teeAt := strings.Index(nmDispatcherScriptFor("test"), `} | $as_user tee "$config_file"`)
	if grepAt < 0 || teeAt < 0 {
		t.Fatal("dispatcher is missing the address-record read or the tee pipeline")
	}
	if grepAt > teeAt {
		t.Error("address records must be read before the tee pipeline truncates the file")
	}
}

// lerd0 is unmanaged, so NM never dispatches for it. The script should bail out
// early rather than trying to read upstream DNS off a link that has none.
func TestNMDispatcherScript_ignoresDummyLink(t *testing.T) {
	assertContains(t, nmDispatcherScriptFor("test"), `if [ "$IFACE" = "lerd0" ]; then`)
	if strings.Contains(nmDispatcherScriptFor("test"), "resolvectl dns lerd0") {
		t.Error("lerd-dns-link.service owns lerd0's route; the dispatcher must not set it")
	}
}

// lerd owns lerd0 through a system unit and tells NM to leave it alone. An
// NM-managed connection appears as a togglable network in the desktop's network
// menu, where switching it off silently breaks offline .test resolution.
func TestLerdLinkUnit_shape(t *testing.T) {
	assertContains(t, lerdNMUnmanagedContent, "unmanaged-devices=interface-name:lerd0")
	assertContains(t, lerdLinkUnitContentFor("test"), "ip link add lerd0 type dummy")
	assertContains(t, lerdLinkUnitContentFor("test"), "resolvectl domain lerd0 ~test")
	assertContains(t, lerdLinkUnitContentFor("test"), "After=systemd-resolved.service")
	assertContains(t, lerdLinkUnitContentFor("test"), "WantedBy=multi-user.target")
	assertContains(t, lerdLinkUnitContentFor("test"), "ip link del lerd0")
}

// systemd-resolved only gives a link a DNS scope once it carries a routable
// address. With a link-local address alone lerd0 reports "Current Scopes: none"
// and .test does not resolve offline at all, which defeats the link's purpose.
// The address must come from a range that cannot exist on a real network, so the
// /32 local route can't shadow a host the user needs to reach: RFC 5737
// TEST-NET-1 (192.0.2.0/24) is reserved for documentation and fits exactly.
func TestLerdLinkUnit_carriesReservedAddress(t *testing.T) {
	assertContains(t, lerdLinkUnitContentFor("test"), "ip addr replace "+lerdDummyAddr+" dev lerd0")
	if !strings.HasPrefix(lerdDummyAddr, "192.0.2.") {
		t.Errorf("lerd0 address %q must come from RFC 5737 TEST-NET-1, which never appears on a real network", lerdDummyAddr)
	}
	if !strings.HasSuffix(lerdDummyAddr, "/32") {
		t.Errorf("lerd0 address %q must be a /32 so it claims exactly one address", lerdDummyAddr)
	}
}

// lerd0 must carry ~test only, never ~.: with ~. every non-.test query offline
// would be funnelled through lerd-dns into a then-unreachable upstream and stall.
func TestLerdLinkUnit_routesTestDomainOnly(t *testing.T) {
	if strings.Contains(lerdLinkUnitContentFor("test"), "~test ~.") {
		t.Error("lerd0 must carry ~test only (~. would funnel all DNS through lerd-dns offline)")
	}
}

// The watcher reapplies DNS config headless, so every privileged step must be
// granted passwordless or it blocks on a prompt no one can answer.
func TestLinuxSudoers_grantsDummyLinkOps(t *testing.T) {
	content := renderLinuxSudoers("alice")
	for _, want := range []string{
		"/usr/bin/tee /etc/systemd/system/lerd-dns-link.service",
		"/usr/bin/chmod 644 /etc/systemd/system/lerd-dns-link.service",
		"/usr/bin/tee /etc/NetworkManager/conf.d/lerd-dns-link.conf",
		"/usr/bin/chmod 644 /etc/NetworkManager/conf.d/lerd-dns-link.conf",
		"/usr/bin/systemctl daemon-reload",
		"/usr/bin/systemctl enable --now lerd-dns-link.service",
		"/usr/bin/systemctl restart lerd-dns-link.service",
		"/usr/bin/systemctl reload NetworkManager",
		"/usr/bin/nmcli connection delete lerd-dns",
	} {
		assertContains(t, content, want)
	}
}

// sudo matches a rule against the literal path secure_path resolves the command
// to, without following symlinks: on Ubuntu that is /usr/sbin/ip, which the
// /usr/bin rule did not match even though it is a symlink to exactly that file,
// so Teardown's link delete prompted for a password instead of running granted.
func TestLinuxSudoers_grantsIpFromEveryLocationItShipsIn(t *testing.T) {
	content := renderLinuxSudoers("alice")
	for _, want := range []string{
		"/usr/bin/ip link del lerd0",  // Arch, Fedora
		"/usr/sbin/ip link del lerd0", // Debian, Ubuntu
		"/sbin/ip link del lerd0",
	} {
		assertContains(t, content, want)
	}
}

// A sudoers drop-in with no %s substituted for the user parses as a rule for a
// literal "%s" user and grants nothing, so guard the format-arg count.
func TestLinuxSudoers_everyRuleNamesTheUser(t *testing.T) {
	content := renderLinuxSudoers("alice")
	if strings.Contains(content, "%!s(MISSING)") || strings.Contains(content, "%!(EXTRA") {
		t.Fatalf("sudoers format args mismatch:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "NOPASSWD") {
			continue
		}
		if !strings.HasPrefix(line, "alice ALL=(root) NOPASSWD: /") {
			t.Errorf("malformed sudoers rule: %q", line)
		}
	}
}

// --- WriteDnsmasqConfigFor ---

func TestWriteDnsmasqConfigFor_customTarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 127.0.0.53\n")
	origPaths := resolvPaths
	origNmcli := nmcliDNSFunc
	resolvPaths = []string{fakeResolv}
	nmcliDNSFunc = func() []string { return nil }
	defer func() { resolvPaths = origPaths; nmcliDNSFunc = origNmcli }()

	if err := WriteDnsmasqConfigFor(dir, "10.0.0.5"); err != nil {
		t.Fatalf("WriteDnsmasqConfigFor: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))
	assertContains(t, content, "address=/.test/10.0.0.5")
	assertContains(t, content, "no-resolv")
	assertContains(t, content, "server="+pastaDefaultForwarder)
}

func TestWriteDnsmasqConfigFor_emptyTargetDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 127.0.0.53\n")
	origPaths := resolvPaths
	origNmcli := nmcliDNSFunc
	resolvPaths = []string{fakeResolv}
	nmcliDNSFunc = func() []string { return nil }
	defer func() { resolvPaths = origPaths; nmcliDNSFunc = origNmcli }()

	if err := WriteDnsmasqConfigFor(dir, ""); err != nil {
		t.Fatalf("WriteDnsmasqConfigFor: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))
	assertContains(t, content, "address=/.test/127.0.0.1")
}

// --- v6 dnsmasq output ---

func TestWriteDnsmasqConfig_emitsV6Listen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 8.8.8.8\n")
	origPaths := resolvPaths
	resolvPaths = []string{fakeResolv}
	defer func() { resolvPaths = origPaths }()

	if err := WriteDnsmasqConfig(dir); err != nil {
		t.Fatalf("WriteDnsmasqConfig: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))
	assertContains(t, content, "address=/.test/127.0.0.1")
	assertContains(t, content, "address=/.test/::1")
}

func TestWriteDnsmasqConfigDual_skipsV6WhenEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	fakeResolv := writeTempFile(t, "nameserver 8.8.8.8\n")
	origPaths := resolvPaths
	resolvPaths = []string{fakeResolv}
	defer func() { resolvPaths = origPaths }()

	if err := WriteDnsmasqConfigDual(dir, "10.0.0.5", ""); err != nil {
		t.Fatalf("WriteDnsmasqConfigDual: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "lerd.conf"))
	assertContains(t, content, "address=/.test/10.0.0.5")
	if strings.Contains(content, "address=/.test/::") {
		t.Errorf("expected no v6 address record when v6Target empty, got:\n%s", content)
	}
}

func TestDeriveV6Target(t *testing.T) {
	cases := []struct {
		v4   string
		want string
	}{
		{"", "::1"},
		{"127.0.0.1", "::1"},
	}
	for _, c := range cases {
		if got := deriveV6Target(c.v4); got != c.want {
			t.Errorf("deriveV6Target(%q) = %q, want %q", c.v4, got, c.want)
		}
	}
	// A LAN target derives to the host's global v6 when it has one, or ""
	// (no AAAA record) when it doesn't. It must never fall back to ::1, which
	// would wrongly answer remote AAAA queries with loopback. We can't pin the
	// exact value (host-dependent), but it must not be ::1.
	if got := deriveV6Target("10.0.0.5"); got == "::1" {
		t.Error("deriveV6Target(LAN) must not fall back to ::1")
	}
}

// --- lerdDNSInterfaces parsing ---

func TestLerdDNSInterfaces_multipleLinks(t *testing.T) {
	output := `Global
           Protocols: +LLMNR +mDNS
    resolv.conf mode: foreign

Link 2 (enp14s0)
    Current Scopes: DNS
Current DNS Server: 192.168.0.151
       DNS Servers: 192.168.0.151

Link 3 (wlan0)
    Current Scopes: none

Link 4 (virbr0)
    Current Scopes: DNS
Current DNS Server: 127.0.0.1:5300
       DNS Servers: 127.0.0.1:5300
        DNS Domain: ~test ~.

Link 6 (vnet1)
    Current Scopes: DNS
Current DNS Server: 127.0.0.1:5300
       DNS Servers: 127.0.0.1:5300
        DNS Domain: ~test ~.
`
	ifaces := parseLerdDNSInterfaces(output)
	want := []string{"virbr0", "vnet1"}
	assertSliceEqual(t, ifaces, want)
}

func TestLerdDNSInterfaces_none(t *testing.T) {
	output := `Link 2 (enp14s0)
Current DNS Server: 192.168.0.151
       DNS Servers: 192.168.0.151
`
	ifaces := parseLerdDNSInterfaces(output)
	if len(ifaces) != 0 {
		t.Errorf("expected empty, got %v", ifaces)
	}
}

// --- ResolverHint ---

func TestResolverHint_NetworkManager(t *testing.T) {
	origNM := isNetworkManagerActive
	origResolved := isSystemdResolvedActive
	defer func() { isNetworkManagerActive = origNM; isSystemdResolvedActive = origResolved }()

	isNetworkManagerActive = func() bool { return true }
	isSystemdResolvedActive = func() bool { return true }

	got := ResolverHint()
	if got != "sudo systemctl restart NetworkManager" {
		t.Errorf("expected NM hint, got %q", got)
	}
}

func TestResolverHint_SystemdResolvedOnly(t *testing.T) {
	origNM := isNetworkManagerActive
	origResolved := isSystemdResolvedActive
	defer func() { isNetworkManagerActive = origNM; isSystemdResolvedActive = origResolved }()

	isNetworkManagerActive = func() bool { return false }
	isSystemdResolvedActive = func() bool { return true }

	got := ResolverHint()
	if got != "sudo systemctl restart systemd-resolved" {
		t.Errorf("expected systemd-resolved hint, got %q", got)
	}
}

func TestResolverHint_NoResolver(t *testing.T) {
	origNM := isNetworkManagerActive
	origResolved := isSystemdResolvedActive
	defer func() { isNetworkManagerActive = origNM; isSystemdResolvedActive = origResolved }()

	isNetworkManagerActive = func() bool { return false }
	isSystemdResolvedActive = func() bool { return false }

	got := ResolverHint()
	if got != "restart your DNS resolver" {
		t.Errorf("expected generic hint, got %q", got)
	}
}

func TestSetupNetworkManager_missingDnsmasqBinaryFailsFast(t *testing.T) {
	origPresent := dnsmasqBinaryPresent
	defer func() { dnsmasqBinaryPresent = origPresent }()
	dnsmasqBinaryPresent = func() bool { return false }

	err := setupNetworkManager()
	if err == nil {
		t.Fatal("expected an error when dnsmasq is not on PATH, got nil")
	}
	if !strings.Contains(err.Error(), "dnsmasq") {
		t.Errorf("error %q should mention dnsmasq", err.Error())
	}
}

// --- helpers (Linux-only) ---

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A host that upgraded the binary without re-running install still has the old
// sudoers grants, and the watcher reapplies DNS config headless where a password
// prompt would hang forever. The grants are probed before any privileged step,
// and a stale drop-in downgrades to a warning rather than failing the start:
// online .test still resolves over the per-interface route either way.
func TestSetupDummyLink_skipsWhenSudoersGrantsAreStale(t *testing.T) {
	orig := dummyLinkGrantsLive
	t.Cleanup(func() { dummyLinkGrantsLive = orig })

	probed := false
	dummyLinkGrantsLive = func() bool { probed = true; return false }

	if err := setupDummyLink(true, "test"); err == nil {
		t.Fatal("setupDummyLink must report an error when the grants are stale, or callers remove the hookup it was meant to replace")
	}
	if !probed {
		t.Error("setupDummyLink must probe the sudoers grants before running any privileged step")
	}
}

// On a host with no NetworkManager (Arch/omarchy: resolved + networkd + iwd),
// the unmanaged rule is pointless, and writing it would create an
// /etc/NetworkManager tree on a machine that has no NetworkManager at all.
func TestDummyLinkNMRuleNeeded_neverWithoutNetworkManager(t *testing.T) {
	if dummyLinkNMRuleNeeded(false) {
		t.Error("must not write the NetworkManager rule on a host without NetworkManager")
	}
}

// Enablement and health are different questions, and conflating them loses lerd0
// at the next boot: a link that happens to be up right now (left over, or made by
// hand) must not stop the unit being enabled, because enabled is the only thing
// that brings it back after a reboot.
func TestEnsureDummyLinkRunning_checksEnablementEvenWhenLinkAlreadyUp(t *testing.T) {
	origHealthy, origEnabled := dummyLinkHealthy, dummyLinkUnitEnabled
	t.Cleanup(func() { dummyLinkHealthy, dummyLinkUnitEnabled = origHealthy, origEnabled })

	dummyLinkHealthy = func(string) bool { return true } // link is up right now
	askedEnabled := false
	// Report enabled so no privileged command runs during the test.
	dummyLinkUnitEnabled = func() bool { askedEnabled = true; return true }

	ensureDummyLinkRunning("test")

	if !askedEnabled {
		t.Error("enablement must be checked even when the link is already healthy, or lerd0 is gone after the next reboot")
	}
}

// lerd0 is what stops resolved answering "Network is down" instantly while
// offline, which is the point for .test, but the same flag makes resolved chase
// unreachable fallback servers for every other name, hanging each offline lookup
// for 20s+. Turning the fallbacks off is the only lever that removes the hang,
// and it is a no-op on Debian, Ubuntu and Fedora, which ship them off already.
func TestLerdFallbackDropin_disablesFallbackServers(t *testing.T) {
	assertContains(t, lerdFallbackDropinContent, "[Resolve]")
	assertContains(t, lerdFallbackDropinContent, "FallbackDNS=")
	// A value here would set fallbacks rather than clear them.
	for _, line := range strings.Split(lerdFallbackDropinContent, "\n") {
		if strings.HasPrefix(line, "FallbackDNS=") && strings.TrimSpace(line) != "FallbackDNS=" {
			t.Errorf("FallbackDNS must be cleared, not assigned: %q", line)
		}
	}
	if !strings.HasSuffix(lerdFallbackDropin, ".conf") || !strings.Contains(lerdFallbackDropin, "/etc/systemd/resolved.conf.d/") {
		t.Errorf("fallback drop-in %q must live in resolved.conf.d", lerdFallbackDropin)
	}
	// Must not collide with the drop-in the no-NetworkManager path writes, which
	// setupNMWithResolved deletes as a stale artefact.
	if lerdFallbackDropin == "/etc/systemd/resolved.conf.d/lerd.conf" {
		t.Error("fallback drop-in must not reuse the resolver drop-in's path; the NM path deletes that file")
	}
}

// Turning the fallbacks off is lerd's doing and only justified while lerd0 is
// there. Leaving the drop-in behind on uninstall would silently keep the user's
// DNS changed forever, so Teardown has to take it back out.
func TestTeardown_removesFallbackDropin(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	_, teardown, found := strings.Cut(string(src), "func Teardown()")
	if !found {
		t.Fatal("Teardown not found in setup.go")
	}
	if !strings.Contains(teardown, "restoreResolvedFallbacks()") {
		t.Error("Teardown must remove the fallback drop-in, or uninstalling lerd leaves the system's fallback DNS off for good")
	}
}

// Left behind, /etc/sudoers.d/lerd is a standing NOPASSWD root grant (resolvectl,
// a root-run NM dispatcher script, restarting NetworkManager) for a tool being
// removed, so Teardown must delete it. It has to go last, once nothing above
// still depends on the grants it provides.
func TestTeardown_removesTheSudoersGrant(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	_, teardown, found := strings.Cut(string(src), "func Teardown()")
	if !found {
		t.Fatal("Teardown not found in setup.go")
	}
	if !strings.Contains(teardown, "removeSudoersGrant()") {
		t.Fatal("Teardown must remove /etc/sudoers.d/lerd, or uninstalling lerd leaves a passwordless root grant behind")
	}
	sudoersAt := strings.Index(teardown, "removeSudoersGrant()")
	resolverAt := strings.LastIndex(teardown, `"restart", "systemd-resolved"`)
	nmAt := strings.LastIndex(teardown, `"restart", "NetworkManager"`)
	last := resolverAt
	if nmAt > last {
		last = nmAt
	}
	if last >= 0 && sudoersAt < last {
		t.Error("the sudoers removal must come after the granted resolver/NetworkManager restarts, or it revokes the grants those steps still need")
	}
}

// The write is guarded on the link coming up, but nothing ever walked it back. A
// host that had lerd0 and lost it (dummy module dropped by a kernel update, unit
// removed by hand) keeps the drop-in from the run that worked, and is left with
// resolved's fallbacks off and no lerd0: exactly the state the guard exists to
// prevent, and strictly worse than never having touched the fallbacks at all.
func TestSetupDummyLink_handsTheFallbacksBackWhenTheLinkStopsWorking(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	body := string(src)

	fn := section(t, body, "func setupDummyLink(", "// restoreResolvedFallbacks")
	restoreAt := strings.Index(fn, "restoreResolvedFallbacks()")
	writeAt := strings.Index(fn, "if fallbackChanged {")
	if restoreAt < 0 {
		t.Fatal("setupDummyLink must restore the fallbacks when the link cannot be brought up")
	}
	if writeAt < 0 || restoreAt > writeAt {
		t.Error("the restore belongs on the ensureDummyLinkRunning failure path, before the drop-in is written")
	}

	restore := section(t, body, "func restoreResolvedFallbacks() {", "\n}")
	// Unguarded this prompts for a password on every host that never had the
	// drop-in, which is most of them.
	assertContains(t, restore, "os.Stat(lerdFallbackDropin)")
	assertContains(t, restore, `"restart", "systemd-resolved"`)

	// It used to run only from an interactive `lerd uninstall`; on the start path
	// the watcher runs it headless, where an ungranted command is a prompt nobody
	// can answer and the start hangs forever.
	sudoers := renderLinuxSudoers("alice")
	assertContains(t, sudoers, "/usr/bin/rm -f /etc/systemd/resolved.conf.d/lerd-fallback.conf")
	assertContains(t, sudoers, "/usr/bin/systemctl restart systemd-resolved")
}

// The grants-out-of-date early return must hand the fallbacks back too. An
// upgrade that adds a required grant lands here until `lerd install` runs, and a
// host that already had lerd0 still carries the fallback drop-in, so bailing
// without restoring leaves resolved's fallbacks off with no lerd0, hanging every
// offline lookup, with no later run repairing it because each takes the same
// early return.
func TestSetupDummyLink_handsTheFallbacksBackWhenTheGrantsAreStale(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	guard := section(t, string(src), "if !dummyLinkGrantsLive() {", "// Migrate hosts")
	assertContains(t, guard, "restoreResolvedFallbacks()")
	restoreAt := strings.Index(guard, "restoreResolvedFallbacks()")
	returnAt := strings.Index(guard, "return fmt.Errorf")
	if returnAt < 0 || restoreAt > returnAt {
		t.Error("the restore must run before the early return, not after it")
	}
}

// The link and everything that reads it must agree on the TLD. lerd supports a
// custom dns.tld, and hardcoding "test" in the unit means lerd0 carries a route
// for a domain the user does not use: offline .tld resolution silently does
// nothing for them, and the diagnostic that checks it warns forever.
func TestLerdLinkUnit_usesTheConfiguredTLD(t *testing.T) {
	unit := lerdLinkUnitContentFor("dev")
	assertContains(t, unit, "resolvectl domain lerd0 ~dev")
	assertContains(t, unit, "Description=lerd .dev DNS link")
	if strings.Contains(unit, "~test") {
		t.Error("the unit must carry the configured TLD, not a hardcoded ~test")
	}
}

// The grants probe must answer "does this go through without a password", not
// "is george allowed to sudo at all". `sudo -n -l <cmd>` answers the second, and
// on any normal desktop (george ALL=(ALL) ALL) it succeeds even with no lerd
// grants at all, so the guard could never fire where it was needed.
func TestDummyLinkGrantsLive_runsAGrantedCommandRatherThanAskingSudoL(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	body, _, found := strings.Cut(string(src), "// dummyLinkNMRuleNeeded")
	if !found {
		t.Fatal("could not isolate the probe")
	}
	_, probe, found := strings.Cut(body, "var dummyLinkGrantsLive")
	if !found {
		t.Fatal("dummyLinkGrantsLive not found")
	}
	if strings.Contains(probe, `"-l"`) {
		t.Error(`the probe must not use "sudo -n -l": that reports whether the user MAY run the command, which is true for any sudoer regardless of the NOPASSWD grants`)
	}
	assertContains(t, probe, `"sudo", "-n"`)
}

// Belt and braces: a TLD that passes the pattern lands in the unit as an inert
// word and cannot terminate the quoting around ExecStart's shell command.
func TestLerdLinkUnit_tldLandsInertInTheUnit(t *testing.T) {
	unit := lerdLinkUnitContentFor("my-tld")
	assertContains(t, unit, "resolvectl domain lerd0 ~my-tld'")
	if strings.Count(unit, "ExecStart=") != 1 {
		t.Error("the TLD must not be able to introduce a second ExecStart")
	}
}

// lerd0 is the offline enhancement, never a precondition. Hosts exist where the
// dummy module is absent (the stock WSL2 kernel) or lerd's absolute-path grants
// cannot match (NixOS). Making the link fatal took .tld down entirely on those
// hosts, which is strictly worse than the behaviour it replaced: the baseline
// hookup resolves .tld whenever a link is up, and must be written regardless.
func TestSetupPaths_treatTheLinkAsAnEnhancementNotAPrecondition(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	body := string(src)

	nm := section(t, body, "func setupNMWithResolved() error {", "// setupSystemdResolved")
	if strings.Contains(nm, "if err := setupDummyLink(true, tld); err != nil {\n\t\treturn err") {
		t.Error("setupNMWithResolved must not abort on a link failure: the per-interface route below it is what makes .tld resolve at all")
	}
	assertContains(t, nm, "WARN: offline")

	res := section(t, body, "func setupSystemdResolved() error {", "// writeResolvedDropin")
	// The baseline is written (only when the link is not already up) before the
	// superseding link is attempted, so a host that cannot build the link keeps a
	// working hookup.
	writeAt := strings.Index(res, "writeResolvedDropin(dropin, tld)")
	linkAt := strings.Index(res, "if err := setupDummyLink(false, tld); err != nil {")
	if writeAt < 0 || linkAt < 0 {
		t.Fatal("expected setupSystemdResolved to write the baseline and then try the link")
	}
	if writeAt > linkAt {
		t.Error("the baseline drop-in must be written before the superseding link is attempted, or a fresh host with no link has no hookup at all")
	}
	// The link is best effort here too: a failure warns and returns, leaving the
	// baseline in place rather than aborting.
	assertContains(t, res, "WARN: offline")
	if strings.Contains(res, "setupDummyLink(false, tld); err != nil {\n\t\treturn err") {
		t.Error("setupSystemdResolved must not abort on a link failure; the baseline still resolves .tld while a link is up")
	}

	// The removal is idempotent: guarded by a stat so it retries a once-failed
	// removal on a later run instead of skipping it forever, and only after the
	// link is confirmed.
	rem := section(t, body, "func removeSupersededResolvedDropin(", "\n}")
	assertContains(t, rem, "os.Stat(dropin)")
	rmAt := strings.Index(rem, `"rm", "-f", dropin`)
	reapplyAt := strings.Index(rem, "ensureDummyLinkRunning(tld)")
	if rmAt < 0 || reapplyAt < 0 || rmAt > reapplyAt {
		t.Error("removeSupersededResolvedDropin must remove then reapply the route")
	}
	// On a reapply failure it restores the baseline rather than leaving the host bare.
	assertContains(t, rem, "writeResolvedDropin(dropin, tld)")
}

// dns:disable flips the TLD to localhost and stops lerd-dns. Carrying on into
// resolver setup there prompts for a password on a host that opted out and points
// a ~localhost route at a container that is deliberately not running.
func TestConfigureResolver_doesNothingWhenDNSIsDisabled(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	fn := section(t, string(src), "func ConfigureResolver() error {", "func setupDummyLink")
	assertContains(t, fn, "!cfg.DNS.Enabled")
	if strings.Index(fn, "!cfg.DNS.Enabled") > strings.Index(fn, "isSystemdResolvedActive()") {
		t.Error("the disabled check must come before any resolver path runs")
	}
	assertContains(t, fn, "HostOwnsResolver()")
	if strings.Index(fn, "HostOwnsResolver()") > strings.Index(fn, "isSystemdResolvedActive()") {
		t.Error("the NixOS host-owns-resolver check must come before any resolver path runs")
	}
}

func TestConfigureResolver_doesNothingWhenHostOwnsResolver(t *testing.T) {
	orig := HostOwnsResolver
	t.Cleanup(func() { HostOwnsResolver = orig })
	HostOwnsResolver = func() bool { return true }

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	if err := ConfigureResolver(); err != nil {
		t.Fatalf("ConfigureResolver: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("ConfigureResolver NixOS path must stay silent, got %q", buf.String())
	}
}

// The watcher calls ConfigureResolver whenever .test fails. A Note there would
// spam (or mislead) on every repair; the once-per-run explanation lives on the
// interactive install and start paths instead.
func TestConfigureResolver_hostOwnsResolverPathDoesNotPrint(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	fn := section(t, string(src), "func ConfigureResolver() error {", "func setupDummyLink")
	if strings.Contains(fn, "NoteNixOSOwnsResolver") {
		t.Error("ConfigureResolver must not call NoteNixOSOwnsResolver; the watcher invokes it on every .test failure")
	}
	i := strings.Index(fn, "if HostOwnsResolver() {")
	if i < 0 {
		t.Fatal("HostOwnsResolver guard missing")
	}
	block := fn[i:]
	if j := strings.Index(block, "return nil"); j >= 0 {
		block = block[:j]
	}
	for _, needle := range []string{"feedback.Note", "feedback.Line", "fmt.Print", "fmt.Printf"} {
		if strings.Contains(block, needle) {
			t.Errorf("ConfigureResolver NixOS return path must stay silent, found %s", needle)
		}
	}
}

func TestNoteNixOSOwnsResolver_mentionsHostOwnsResolver(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	fn := section(t, string(src), "func NoteNixOSOwnsResolver() {", "func ConfigureResolver()")
	assertContains(t, fn, "HostOwnsResolver()")
}

func TestNoteNixOSOwnsResolver_printsOnceWhenHostOwnsResolver(t *testing.T) {
	orig := HostOwnsResolver
	t.Cleanup(func() {
		HostOwnsResolver = orig
		noteNixOSOwnsResolverOnce = sync.Once{}
	})
	noteNixOSOwnsResolverOnce = sync.Once{}
	HostOwnsResolver = func() bool { return true }

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	NoteNixOSOwnsResolver()
	NoteNixOSOwnsResolver()

	got := buf.String()
	if strings.Count(got, "NixOS owns the resolver") != 1 {
		t.Fatalf("got %q, want the NixOS resolver note once", got)
	}
	if !strings.Contains(got, "127.0.0.1:5300") {
		t.Errorf("note should mention 127.0.0.1:5300, got %q", got)
	}
	if !strings.Contains(got, "block #5") {
		t.Errorf("note should point at configuration.nix block #5, got %q", got)
	}
}

func TestNoteNixOSOwnsResolver_silentWhenHostDoesNotOwnResolver(t *testing.T) {
	orig := HostOwnsResolver
	t.Cleanup(func() {
		HostOwnsResolver = orig
		noteNixOSOwnsResolverOnce = sync.Once{}
	})
	noteNixOSOwnsResolverOnce = sync.Once{}
	HostOwnsResolver = func() bool { return false }

	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	NoteNixOSOwnsResolver()
	if buf.Len() != 0 {
		t.Fatalf("non-NixOS path must stay silent, got %q", buf.String())
	}
}

func TestWriteSudoersForUser_skipsWhenHostOwnsResolver(t *testing.T) {
	orig := HostOwnsResolver
	t.Cleanup(func() { HostOwnsResolver = orig })
	HostOwnsResolver = func() bool { return true }
	if err := WriteSudoersForUser("not a valid user"); err != nil {
		t.Fatalf("WriteSudoersForUser: %v", err)
	}
}

func TestInstallSudoers_skipsWhenHostOwnsResolver(t *testing.T) {
	orig := HostOwnsResolver
	t.Cleanup(func() { HostOwnsResolver = orig })
	HostOwnsResolver = func() bool { return true }
	if err := InstallSudoers(); err != nil {
		t.Fatalf("InstallSudoers: %v", err)
	}
}

// section returns the source between two markers, failing the test if either is
// missing so a rename cannot silently turn these guards into no-ops.
func section(t *testing.T, body, from, to string) string {
	t.Helper()
	i := strings.Index(body, from)
	if i < 0 {
		t.Fatalf("marker %q not found", from)
	}
	rest := body[i:]
	j := strings.Index(rest, to)
	if j < 0 {
		t.Fatalf("marker %q not found after %q", to, from)
	}
	return rest[:j]
}

// A link that is up but whose unit never enabled disappears at the next boot,
// which is precisely the property the link exists to provide and the one a health
// check cannot see. Reporting success there hides it until the user reboots.
func TestEnsureDummyLinkRunning_failsWhenTheUnitIsNotEnabled(t *testing.T) {
	origHealthy, origEnabled := dummyLinkHealthy, dummyLinkUnitEnabled
	t.Cleanup(func() { dummyLinkHealthy, dummyLinkUnitEnabled = origHealthy, origEnabled })

	dummyLinkHealthy = func(string) bool { return true } // carrying the route now
	// Report enabled once (so no privileged command runs) and never again, i.e.
	// the enable did not take.
	calls := 0
	dummyLinkUnitEnabled = func() bool { calls++; return calls == 1 }

	if err := ensureDummyLinkRunning("test"); err == nil {
		t.Error("a healthy link with a unit that is not enabled must be reported: it is gone after the next reboot")
	}
}

// Every file lerd removes in Teardown must have an rm grant, or the headless
// watcher prompts and teardown silently leaves the resolver hijacked. Derived
// from resolverArtifacts, the same canonical list ResolverConfigured and Teardown
// use, so a file added there without a grant fails here rather than in the field.
// The hand-listed variant this replaced was green while two NM teardown paths were
// ungranted, which is exactly the gap this enumeration closes.
func TestLinuxSudoers_grantsEveryFileTeardownRemoves(t *testing.T) {
	grants := renderLinuxSudoers("alice")
	for _, path := range resolverArtifacts {
		if !strings.Contains(grants, "/usr/bin/rm -f "+path) {
			t.Errorf("Teardown removes %s but there is no NOPASSWD rm grant for it", path)
		}
	}
}

// The write and control commands the setup paths run must be granted too, since a
// mismatch is a silent prompt in the headless lerd-ui watcher.
func TestLinuxSudoers_grantsEveryPrivilegedCommandTheCodeRuns(t *testing.T) {
	grants := renderLinuxSudoers("alice")
	for _, want := range []string{
		// systemd-resolved drop-in write (setupSystemdResolved)
		"/usr/bin/tee /etc/systemd/resolved.conf.d/lerd.conf",
		"/usr/bin/chmod 644 /etc/systemd/resolved.conf.d/lerd.conf",
		// the link unit, unmanaged rule, fallback drop-in
		"/usr/bin/tee /etc/systemd/system/lerd-dns-link.service",
		"/usr/bin/tee /etc/NetworkManager/conf.d/lerd-dns-link.conf",
		"/usr/bin/tee /etc/systemd/resolved.conf.d/lerd-fallback.conf",
		// NetworkManager embedded-dnsmasq path
		"/usr/bin/tee /etc/NetworkManager/conf.d/lerd.conf",
		"/usr/bin/tee /etc/NetworkManager/dnsmasq.d/lerd.conf",
		"/usr/bin/mkdir -p /etc/NetworkManager/dnsmasq.d",
		"/usr/bin/systemctl restart NetworkManager",
		// link control
		"/usr/bin/systemctl enable --now lerd-dns-link.service",
		"/usr/bin/systemctl disable --now lerd-dns-link.service",
		"/usr/bin/ip link del lerd0",
	} {
		if !strings.Contains(grants, want) {
			t.Errorf("missing NOPASSWD grant, headless watcher would prompt: %s", want)
		}
	}
}

func TestMain(m *testing.M) {
	HostOwnsResolver = func() bool { return false }
	os.Exit(m.Run())
}

// resolvectl ships with systemd whether or not resolved runs it, so on a host
// where NetworkManager owns resolv.conf the teardown's revert and DNS push
// reach for a service that is not there and fail with "Could not activate
// remote peer org.freedesktop.resolve1". Nothing is actually wrong, but an
// uninstall that prints two red lines reads as a broken one.
func TestResolvedInterfacesToRevert_EmptyWithoutResolved(t *testing.T) {
	orig := isSystemdResolvedActive
	t.Cleanup(func() { isSystemdResolvedActive = orig })

	isSystemdResolvedActive = func() bool { return false }
	if got := resolvedInterfacesToRevert(); got != nil {
		t.Errorf("interfaces = %v, want none: there is no resolved to revert them in", got)
	}
}

// The DNS push after the NetworkManager restart is resolved's too, so it has
// to be gated the same way.
func TestTeardown_pushesDNSOnlyWhereResolvedRuns(t *testing.T) {
	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("reading setup.go: %v", err)
	}
	_, teardown, found := strings.Cut(string(src), "func Teardown()")
	if !found {
		t.Fatal("Teardown not found in setup.go")
	}
	if !strings.Contains(teardown, `iface := defaultInterface(); iface != "" && isSystemdResolvedActive()`) {
		t.Error("the resolvectl dns push must be gated on resolved running, or it fails loudly on a NetworkManager host")
	}
	if !strings.Contains(teardown, "resolvedInterfacesToRevert()") {
		t.Error("the revert loop must go through the resolved-gated lister")
	}
}
