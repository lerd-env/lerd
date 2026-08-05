package dns

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/geodro/lerd/pkg/distro"
)

// fakeProbes builds a probeFns where every rung is overridable but defaults
// to "everything passes". Tests then flip individual rungs to exercise
// failure paths.
func fakeProbes() probeFns {
	return probeFns{
		containerRunning:   func() bool { return true },
		dnsmasqConfigOK:    func(string) (bool, string) { return true, "ok" },
		portOpen:           func(string, int) bool { return true },
		dnsmasqAnswer:      func(string) (string, error) { return "127.0.0.1", nil },
		resolverHookup:     func() (string, bool, string) { return "drop-in", true, "/etc/x" },
		interfaceRouting:   func(string) (string, bool, bool, error) { return "eth0", true, true, nil },
		dummyLinkRouting:   func(string) (bool, bool) { return true, true },
		systemLookup:       func(string) ([]string, error) { return []string{"127.0.0.1"}, nil },
		vpnActive:          func() bool { return false },
		lanExposedIP:       func() string { return "" },
		hostDnsmasqPresent: func() bool { return true },
	}
}

// nmProbes is fakeProbes on the NetworkManager + systemd-resolved path, the only
// one that provisions lerd0 and so the only one the offline-route rung runs on.
func nmProbes() probeFns {
	p := fakeProbes()
	p.resolverHookup = func() (string, bool, string) {
		return nmDispatcherKind, true, "/etc/NetworkManager/dispatcher.d/99-lerd-dns"
	}
	return p
}

// findStep returns the step whose name contains sub, or nil.
func findStep(d Diagnostic, sub string) *Step {
	for i := range d.Steps {
		if strings.Contains(d.Steps[i].Name, sub) {
			return &d.Steps[i]
		}
	}
	return nil
}

func TestDiagnose_allOK(t *testing.T) {
	d := diagnose("test", fakeProbes())
	if d.FirstFailure != -1 {
		t.Errorf("FirstFailure = %d, want -1 when every rung passes", d.FirstFailure)
	}
	for _, s := range d.Steps {
		if s.Status != StepOK {
			t.Errorf("step %q = %s (%s), want ok", s.Name, s.Status, s.Detail)
		}
	}
}

func TestDiagnose_containerDownAndPortClosedStopsChain(t *testing.T) {
	p := fakeProbes()
	p.containerRunning = func() bool { return false }
	p.portOpen = func(string, int) bool { return false }
	d := diagnose("test", p)
	if len(d.Steps) != 1 {
		t.Fatalf("expected 1 step (no chain after container fail), got %d", len(d.Steps))
	}
	if d.Steps[0].Status != StepFail {
		t.Errorf("container step status = %s, want fail", d.Steps[0].Status)
	}
	if d.FirstFailure != 0 {
		t.Errorf("FirstFailure = %d, want 0", d.FirstFailure)
	}
	if !strings.Contains(d.Steps[0].Hint, "lerd start") {
		t.Errorf("hint %q should mention `lerd start`", d.Steps[0].Hint)
	}
}

func TestDiagnose_legacyHostResolverDetectedAsWarn(t *testing.T) {
	// Field-report scenario: user has Homebrew/launchd dnsmasq holding
	// :5300, lerd-dns container is absent. The chain should surface this
	// as a WARN (not a hard fail) with a hint explaining the situation.
	p := fakeProbes()
	p.containerRunning = func() bool { return false }
	// portOpen / dnsmasqAnswer default to "yes, answers correctly".
	d := diagnose("test", p)
	if len(d.Steps) != 1 {
		t.Fatalf("expected exactly 1 step (chain stops at the warn), got %d: %+v", len(d.Steps), d.Steps)
	}
	step := d.Steps[0]
	if step.Status != StepWarn {
		t.Errorf("step status = %s, want warn", step.Status)
	}
	if d.FirstFailure != -1 {
		t.Errorf("FirstFailure = %d, want -1 (no failure, lerd just isn't managing DNS)", d.FirstFailure)
	}
	for _, want := range []string{"host-side resolver", "not managing DNS", "lerd start"} {
		if !strings.Contains(step.Detail+step.Hint, want) {
			t.Errorf("step detail+hint should mention %q\ndetail: %s\nhint: %s", want, step.Detail, step.Hint)
		}
	}
}

func TestDiagnose_portSquattedByNonDNSReportsFail(t *testing.T) {
	// Something is on :5300 but it isn't a working DNS resolver (e.g. a
	// stale process). Distinct from the legacy-resolver case so the user
	// gets a different hint, and the actual error is preserved in Detail
	// so the user can tell NXDOMAIN apart from a timeout or refused.
	p := fakeProbes()
	p.containerRunning = func() bool { return false }
	p.dnsmasqAnswer = func(string) (string, error) { return "", errors.New("nxdomain") }
	d := diagnose("test", p)
	if d.FirstFailure != 0 {
		t.Errorf("FirstFailure = %d, want 0 (rung 1 fails when port is squatted by non-DNS)", d.FirstFailure)
	}
	if d.Steps[0].Status != StepFail {
		t.Errorf("step status = %s, want fail", d.Steps[0].Status)
	}
	if !strings.Contains(d.Steps[0].Hint, "identify the holder") {
		t.Errorf("hint should suggest identifying the squatter, got %q", d.Steps[0].Hint)
	}
	if !strings.Contains(d.Steps[0].Detail, "nxdomain") {
		t.Errorf("Detail should surface the actual probe error, got %q", d.Steps[0].Detail)
	}
}

func TestDiagnose_legacyResolverPointingAtNon127ReportsWarn(t *testing.T) {
	// User runs a host dnsmasq mapping .test to a LAN IP (e.g. for
	// cross-device testing). Still a working resolver, still not lerd's,
	// but the IP differs from lerd's default. Surface as WARN with the
	// actual answer included so the user can confirm intent.
	p := fakeProbes()
	p.containerRunning = func() bool { return false }
	p.dnsmasqAnswer = func(string) (string, error) { return "192.168.1.20", nil }
	d := diagnose("test", p)
	if len(d.Steps) != 1 {
		t.Fatalf("expected exactly 1 step, got %d: %+v", len(d.Steps), d.Steps)
	}
	if d.Steps[0].Status != StepWarn {
		t.Errorf("step status = %s, want warn", d.Steps[0].Status)
	}
	if d.FirstFailure != -1 {
		t.Errorf("FirstFailure = %d, want -1 (not a failure, lerd just isn't the resolver)", d.FirstFailure)
	}
	if !strings.Contains(d.Steps[0].Detail, "192.168.1.20") {
		t.Errorf("Detail should mention the actual IP the host resolver returned, got %q", d.Steps[0].Detail)
	}
	if !strings.Contains(d.Steps[0].Detail, "lerd's default is 127.0.0.1") {
		t.Errorf("Detail should call out the difference from lerd's default, got %q", d.Steps[0].Detail)
	}
}

func TestDiagnose_portClosedStopsChain(t *testing.T) {
	p := fakeProbes()
	p.portOpen = func(string, int) bool { return false }
	d := diagnose("test", p)
	if d.FirstFailure != 2 {
		t.Errorf("FirstFailure = %d, want 2 (port rung)", d.FirstFailure)
	}
	hint := d.Steps[2].Hint
	if !strings.Contains(hint, "ss -tlnp") && !strings.Contains(hint, "lsof") {
		t.Errorf("hint %q should suggest ss/lsof for the bound port", hint)
	}
}

func TestDiagnose_wrongAnswerSurfaceConfigDrift(t *testing.T) {
	p := fakeProbes()
	p.dnsmasqAnswer = func(string) (string, error) { return "1.2.3.4", nil }
	d := diagnose("test", p)
	if d.FirstFailure != 3 {
		t.Errorf("FirstFailure = %d, want 3 (dig rung)", d.FirstFailure)
	}
	if !strings.Contains(d.Steps[3].Detail, "1.2.3.4") {
		t.Errorf("detail should include the wrong answer, got %q", d.Steps[3].Detail)
	}
}

// Under lan:expose dnsmasq answers the host LAN IP, not loopback, and the
// system resolver returns it too. The doctor must accept that as healthy
// instead of red-flagging a working setup.
func TestDiagnose_lanExposedAcceptsLANIP(t *testing.T) {
	const lanIP = "192.168.1.50"
	p := fakeProbes()
	p.lanExposedIP = func() string { return lanIP }
	p.dnsmasqAnswer = func(string) (string, error) { return lanIP, nil }
	p.systemLookup = func(string) ([]string, error) { return []string{lanIP}, nil }
	d := diagnose("test", p)
	if d.FirstFailure != -1 {
		t.Errorf("FirstFailure = %d, want -1 (LAN IP is valid under lan:expose)", d.FirstFailure)
	}
	if got := d.Steps[3].Detail; got != lanIP {
		t.Errorf("dig rung detail = %q, want the LAN IP %q", got, lanIP)
	}
}

// The dig-rung hint must point at a cross-platform command. systemctl is
// Linux-only and used to leak into the hint shown on macOS.
func TestDiagnose_digHintUsesDnsRepairNotSystemctl(t *testing.T) {
	p := fakeProbes()
	p.dnsmasqAnswer = func(string) (string, error) { return "1.2.3.4", nil }
	hint := diagnose("test", p).Steps[3].Hint
	if !strings.Contains(hint, "lerd dns:repair") {
		t.Errorf("hint %q should suggest `lerd dns:repair`", hint)
	}
	if strings.Contains(hint, "systemctl") {
		t.Errorf("hint %q must not suggest systemctl (Linux-only)", hint)
	}
}

func TestDiagnose_resolverHookupMissingHintsInstall(t *testing.T) {
	p := fakeProbes()
	p.resolverHookup = func() (string, bool, string) { return "", false, "" }
	d := diagnose("test", p)
	if d.FirstFailure != 4 {
		t.Errorf("FirstFailure = %d, want 4 (resolver hookup rung)", d.FirstFailure)
	}
	if !strings.Contains(d.Steps[4].Hint, "lerd install") {
		t.Errorf("hint %q should suggest lerd install", d.Steps[4].Hint)
	}
}

func TestDiagnose_nmDnsmasqBinaryMissingReportsFail(t *testing.T) {
	p := fakeProbes()
	p.resolverHookup = func() (string, bool, string) {
		return nmDnsmasqKind, true, "/etc/NetworkManager/dnsmasq.d/lerd.conf"
	}
	p.hostDnsmasqPresent = func() bool { return false }
	d := diagnose("test", p)
	step := findStep(d, "resolver hookup")
	if step == nil || step.Status != StepFail {
		t.Fatalf("resolver hookup step = %+v, want fail", step)
	}
	if !strings.Contains(step.Hint, "dnsmasq") {
		t.Errorf("hint %q should mention installing dnsmasq", step.Hint)
	}
}

func TestDiagnose_nmDnsmasqBinaryPresentReportsOK(t *testing.T) {
	p := fakeProbes()
	p.resolverHookup = func() (string, bool, string) {
		return nmDnsmasqKind, true, "/etc/NetworkManager/dnsmasq.d/lerd.conf"
	}
	p.hostDnsmasqPresent = func() bool { return true }
	d := diagnose("test", p)
	step := findStep(d, "resolver hookup")
	if step == nil || step.Status != StepOK {
		t.Errorf("resolver hookup step = %+v, want ok", step)
	}
}

func TestDnsmasqFound_onPath(t *testing.T) {
	ok := dnsmasqFound(func(string) (string, error) { return "/usr/bin/dnsmasq", nil }, nil)
	if !ok {
		t.Error("dnsmasqFound = false, want true when LookPath succeeds")
	}
}

func TestDnsmasqFound_sbinFallback(t *testing.T) {
	// Debian puts dnsmasq in /usr/sbin, which a normal user's PATH omits;
	// the guard must still see it there.
	dir := t.TempDir()
	bin := filepath.Join(dir, "dnsmasq")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	if !dnsmasqFound(notFound, []string{filepath.Join(dir, "missing"), bin}) {
		t.Error("dnsmasqFound = false, want true via sbin fallback stat")
	}
	if dnsmasqFound(notFound, []string{filepath.Join(dir, "missing")}) {
		t.Error("dnsmasqFound = true, want false when PATH and fallbacks all miss")
	}
	if dnsmasqFound(notFound, []string{dir}) {
		t.Error("dnsmasqFound = true, want false for a directory at the fallback path")
	}
}

func TestDiagnose_systemLookupNotFinalizedAsSkip(t *testing.T) {
	// When everything below systemLookup passes but systemLookup fails,
	// the chain should still complete (no early return) so the user can
	// see that the rest of the pipeline is healthy and only the last rung
	// is broken — that's the "drop-in installed but resolved isn't
	// honouring it" cloud-init / EC2 case from issue #285.
	p := fakeProbes()
	p.systemLookup = func(string) ([]string, error) { return nil, errors.New("connect: connection refused") }
	d := diagnose("test", p)
	if got := len(d.Steps); got < 6 {
		t.Fatalf("expected the full chain even on system-lookup fail, got %d steps", got)
	}
	last := d.Steps[len(d.Steps)-1]
	if last.Status != StepFail {
		t.Errorf("last step = %s, want fail", last.Status)
	}
	if !strings.Contains(last.Hint, "cloud-init") {
		t.Errorf("hint %q should mention cloud-init", last.Hint)
	}
}

// TestDiagnose_systemLookupUnderVPNIsWarn pins the VPN-aware Rung 7: when a
// tunnel is up, the system-resolver path failing is expected and lerd
// recovers on its own, so it must downgrade to a warning rather than a
// failure that would mark the whole chain broken.
func TestDiagnose_systemLookupUnderVPNIsWarn(t *testing.T) {
	p := fakeProbes()
	p.systemLookup = func(string) ([]string, error) { return nil, errors.New("server misbehaving") }
	p.vpnActive = func() bool { return true }
	d := diagnose("test", p)

	last := d.Steps[len(d.Steps)-1]
	if last.Status != StepWarn {
		t.Errorf("system lookup under VPN = %s, want warn", last.Status)
	}
	if d.FirstFailure != -1 {
		t.Errorf("FirstFailure = %d, want -1 (a VPN warn is not a chain failure)", d.FirstFailure)
	}
	if !strings.Contains(last.Hint, "VPN") {
		t.Errorf("hint %q should mention VPN", last.Hint)
	}
}

func TestParseInterfaceRouting(t *testing.T) {
	out := `Global
       Protocols: -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
resolv.conf mode: stub

Link 2 (enp1s0)
    Current Scopes: DNS
         Protocols: +DefaultRoute +LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
Current DNS Server: 127.0.0.1:5300
       DNS Servers: 127.0.0.1:5300 1.1.1.1 8.8.8.8
        DNS Domain: ~test ~.
`
	iface, has5300, hasTLD := parseInterfaceRouting(out, "test")
	if iface != "enp1s0" {
		t.Errorf("iface = %q, want %q", iface, "enp1s0")
	}
	if !has5300 {
		t.Error("expected has5300 = true")
	}
	if !hasTLD {
		t.Error("expected hasTLD = true (~test routing domain)")
	}
}

func TestParseInterfaceRouting_missingDomain(t *testing.T) {
	out := `Link 2 (eth0)
       DNS Servers: 127.0.0.1:5300 1.1.1.1
        DNS Domain: ~. example.com
`
	iface, has5300, hasTLD := parseInterfaceRouting(out, "test")
	if iface != "eth0" {
		t.Errorf("iface = %q", iface)
	}
	if !has5300 {
		t.Error("expected has5300")
	}
	if hasTLD {
		t.Error("expected hasTLD = false when ~test is missing")
	}
}

// Regression: when a later Link block had no 127.0.0.1:5300, the parser
// used to reset sawDomain to false on the new block, clobbering the
// already-true result from the earlier matched interface. Two-link
// fixtures exercise that case.
func TestParseInterfaceRouting_domainPersistsAcrossSubsequentLinks(t *testing.T) {
	out := `Link 2 (enp14s0)
Current DNS Server: 127.0.0.1:5300
       DNS Servers: 127.0.0.1:5300 192.168.0.151
        DNS Domain: ~test ~.

Link 3 (wlan0)
    Current Scopes: none
         Protocols: -DefaultRoute +LLMNR +mDNS

Link 4 (virbr0)
       DNS Servers: 127.0.0.1:5300
`
	iface, has5300, hasTLD := parseInterfaceRouting(out, "test")
	if iface != "enp14s0" {
		t.Errorf("iface = %q, want %q", iface, "enp14s0")
	}
	if !has5300 {
		t.Error("expected has5300")
	}
	if !hasTLD {
		t.Error("expected hasTLD = true (was clobbered by later Link block before fix)")
	}
}

func TestParseInterfaceRouting_no5300(t *testing.T) {
	out := `Link 2 (eth0)
       DNS Servers: 1.1.1.1 8.8.8.8
        DNS Domain: ~.
`
	_, has5300, _ := parseInterfaceRouting(out, "test")
	if has5300 {
		t.Error("expected has5300 = false")
	}
}

// --- offline .test route (lerd0) ---

// A missing lerd0 is a warning, not a failure: .test still resolves while a real
// link is up, and only breaks once the user goes offline. The chain must keep
// walking so the end-to-end rung still reports.
func TestDiagnose_dummyLinkMissingWarnsAndContinues(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lerd0 is provisioned on Linux only")
	}
	p := nmProbes()
	p.dummyLinkRouting = func(string) (bool, bool) { return false, false }
	d := diagnose("test", p)

	step := findStep(d, "offline .test route")
	if step == nil {
		t.Fatal("expected an offline .test route step")
	}
	if step.Status != StepWarn {
		t.Errorf("status = %s, want warn (online resolution is unaffected)", step.Status)
	}
	if !strings.Contains(step.Detail, "lerd0") {
		t.Errorf("detail %q should name the link", step.Detail)
	}
	if d.FirstFailure != -1 {
		t.Errorf("FirstFailure = %d, want -1: a warning must not stop the chain", d.FirstFailure)
	}
	if findStep(d, "system DNS lookup") == nil {
		t.Error("chain should continue to the end-to-end rung after a warning")
	}
}

// lerd0 present but with no ~test route is its own failure mode: the link is up
// so it looks fine, but resolved has nothing to forward .test over when offline.
func TestDiagnose_dummyLinkPresentButUnroutedWarns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lerd0 is provisioned on Linux only")
	}
	p := nmProbes()
	p.dummyLinkRouting = func(string) (bool, bool) { return true, false }
	d := diagnose("test", p)

	step := findStep(d, "offline .test route")
	if step == nil {
		t.Fatal("expected an offline .test route step")
	}
	if step.Status != StepWarn {
		t.Errorf("status = %s, want warn", step.Status)
	}
	if !strings.Contains(step.Hint, lerdLinkUnitName) {
		t.Errorf("hint %q should point at the unit that owns the route", step.Hint)
	}
}

// Only the NM + systemd-resolved path provisions lerd0. On a pure resolved
// drop-in or macOS host the link never exists and reporting on it would be noise.
func TestDiagnose_dummyLinkRungSkippedOffNMPath(t *testing.T) {
	p := fakeProbes() // resolverHookup returns "drop-in", not the NM dispatcher
	called := false
	p.dummyLinkRouting = func(string) (bool, bool) { called = true; return false, false }
	d := diagnose("test", p)

	if called {
		t.Error("lerd0 must not be probed on a non-NetworkManager resolver path")
	}
	if findStep(d, "offline .test route") != nil {
		t.Error("no offline .test route step should appear off the NM path")
	}
}

// resolvectl exits 0 even for a link it doesn't know, printing "No such device"
// to stderr and nothing to stdout. Presence must therefore be read off stdout:
// an exit-code check reports a deleted lerd0 as present-but-unrouted, which sends
// the user chasing a routing problem on an interface that isn't there.
func TestDefaultDummyLinkRouting_parsesPresenceFromStdout(t *testing.T) {
	cases := []struct {
		name            string
		stdout          string
		present, routed bool
	}{
		{
			name:   "missing link prints nothing on stdout",
			stdout: "",
		},
		{
			name: "present and routed",
			stdout: "Link 6 (lerd0)\n" +
				"    Current Scopes: DNS\n" +
				"Current DNS Server: 127.0.0.1:5300\n" +
				"       DNS Servers: 127.0.0.1:5300\n" +
				"        DNS Domain: ~test\n",
			present: true, routed: true,
		},
		{
			name: "present but carrying no route",
			stdout: "Link 6 (lerd0)\n" +
				"    Current Scopes: none\n",
			present: true,
		},
		{
			name: "present with a server but no ~test domain",
			stdout: "Link 6 (lerd0)\n" +
				"       DNS Servers: 127.0.0.1:5300\n",
			present: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			present, routed := parseDummyLinkRouting(tc.stdout, "test")
			if present != tc.present || routed != tc.routed {
				t.Errorf("got (present=%v routed=%v), want (present=%v routed=%v)",
					present, routed, tc.present, tc.routed)
			}
		})
	}
}

// Both systemd-resolved paths depend on lerd0 for offline .test: resolved
// refuses a loopback DNS server once no link is routable whether that server is
// per-link (NetworkManager dispatcher) or global (the drop-in used on Arch and
// omarchy, which run resolved without NetworkManager). NetworkManager's own
// dnsmasq resolves .test without resolved and needs no link.
func TestUsesDummyLink_bothResolvedPaths(t *testing.T) {
	for kind, want := range map[string]bool{
		"NetworkManager dispatcher": true,
		"systemd-resolved link":     true,
		"systemd-resolved drop-in":  true,
		"NetworkManager dnsmasq":    false,
		"macOS native dnsmasq":      false,
		"":                          false,
	} {
		if got := usesDummyLink(kind); got != want {
			t.Errorf("usesDummyLink(%q) = %v, want %v", kind, got, want)
		}
	}
}

// The offline-route rung must report on the no-NetworkManager resolved path too;
// there lerd0 is the entire hookup, so a missing link is the whole failure.
func TestDiagnose_dummyLinkRungRunsOnResolvedLinkPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lerd0 is provisioned on Linux only")
	}
	p := fakeProbes()
	p.resolverHookup = func() (string, bool, string) {
		return resolvedLinkKind, true, "/etc/systemd/system/lerd-dns-link.service"
	}
	p.dummyLinkRouting = func(string) (bool, bool) { return false, false }
	d := diagnose("test", p)

	step := findStep(d, "offline .test route")
	if step == nil {
		t.Fatal("expected the offline route rung to run on the no-NetworkManager resolved path")
	}
	if step.Status != StepWarn {
		t.Errorf("status = %s, want warn", step.Status)
	}
}

// The ~tld route match must be a whole-token match: an unanchored substring
// treats a link carrying "~testbed" as carrying the "test" route, so a broken
// link reports healthy and the diagnostic goes green while offline .test fails.
func TestParseDummyLinkRouting_matchesTheDomainAsAWholeToken(t *testing.T) {
	withServer := "Link 6 (lerd0)\n    Current Scopes: DNS\n       DNS Servers: 127.0.0.1:5300\n"
	if _, routed := parseDummyLinkRouting(withServer+"        DNS Domain: ~testbed\n", "test"); routed {
		t.Error("~testbed must not satisfy the ~test route")
	}
	if _, routed := parseDummyLinkRouting(withServer+"        DNS Domain: ~test\n", "test"); !routed {
		t.Error("~test must satisfy the ~test route")
	}
}

// The full dnsmasq package on the debian family starts its own resolver on :53,
// which collides with the NetworkManager plugin the hint is trying to feed. Only
// dnsmasq-base ships the binary alone, which is what install.sh queues too.
func TestDnsmasqInstallHintPerFamily(t *testing.T) {
	orig := detectDistro
	t.Cleanup(func() { detectDistro = orig })

	cases := []struct {
		name string
		d    *distro.Distro
		want string
	}{
		{"ubuntu", &distro.Distro{ID: "ubuntu"}, "sudo apt install dnsmasq-base"},
		{"mint via ID_LIKE", &distro.Distro{ID: "linuxmint", IDLike: "ubuntu debian"}, "sudo apt install dnsmasq-base"},
		{"fedora", &distro.Distro{ID: "fedora"}, "sudo dnf install dnsmasq"},
		{"cachyos via ID_LIKE", &distro.Distro{ID: "cachyos", IDLike: "arch"}, "sudo pacman -S dnsmasq"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detectDistro = func() (*distro.Distro, error) { return tc.d, nil }
			if got := dnsmasqInstallHint(); got != tc.want {
				t.Errorf("dnsmasqInstallHint() = %q, want %q", got, tc.want)
			}
		})
	}
}

// An unrecognised host still gets a usable hint, and it must never name the
// debian package that breaks .test resolution.
func TestDnsmasqInstallHintFallsBackToEveryFamily(t *testing.T) {
	orig := detectDistro
	t.Cleanup(func() { detectDistro = orig })
	detectDistro = func() (*distro.Distro, error) { return nil, errors.New("no os-release") }

	got := dnsmasqInstallHint()
	if !strings.Contains(got, "dnsmasq-base") || strings.Contains(got, "apt install dnsmasq ") {
		t.Errorf("dnsmasqInstallHint() = %q, want the debian entry to name dnsmasq-base", got)
	}
	for _, mgr := range []string{"apt", "dnf", "pacman"} {
		if !strings.Contains(got, mgr) {
			t.Errorf("dnsmasqInstallHint() = %q, want it to cover %s", got, mgr)
		}
	}
}

// On the NetworkManager dnsmasq path, resolution never goes through
// systemd-resolved, so resolvectl has nothing to say and is usually masked.
// Probing it there turns a healthy install into a warning that reads like a
// raw command failure.
func TestDiagnose_SkipsResolvedRoutingOnNMDnsmasq(t *testing.T) {
	p := fakeProbes()
	p.resolverHookup = func() (string, bool, string) {
		return nmDnsmasqKind, true, "/etc/NetworkManager/dnsmasq.d/lerd.conf"
	}
	p.interfaceRouting = func(string) (string, bool, bool, error) {
		return "", false, false, errors.New("exit status 1")
	}

	d := diagnose("test", p)
	if step := findStep(d, "interface routes"); step != nil {
		t.Errorf("resolved routing was probed on the NM dnsmasq path: %+v", *step)
	}
	if hook := findStep(d, "resolver hookup"); hook == nil || hook.Status != StepOK {
		t.Errorf("resolver hookup should still pass: %+v", hook)
	}
}
