package pfredirect

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func useAnchor(t *testing.T, content string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "lerd")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	prev := AnchorPath
	AnchorPath = path
	t.Cleanup(func() { AnchorPath = prev })
}

func TestRenderRedirectsEachLoopbackPortToItsTarget(t *testing.T) {
	got := Render([]int{80, 443})
	for _, want := range []string{
		"rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 80 -> 127.0.0.1 port 17080\n",
		"rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port 17443\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("anchor missing %q:\n%s", want, got)
		}
	}
}

func TestPortsListsOnlyPrivilegedNginxPorts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got := Ports()
	if len(got) != 2 || got[0] != 80 || got[1] != 443 {
		t.Errorf("Ports() = %v, want [80 443]", got)
	}
}

func TestRedirectedCoversPortsOfTheInstalledAnchor(t *testing.T) {
	useAnchor(t, Render([]int{80, 443}))
	got := Redirected()
	if !got[80] || !got[443] || len(got) != 2 {
		t.Errorf("Redirected() = %v, want 80 and 443", got)
	}
}

// Publishing nginx on its high port with nothing forwarding to it would take
// every site down, so a missing or stale anchor must cover nothing.
func TestRedirectedEmptyWithoutAMatchingAnchor(t *testing.T) {
	useAnchor(t, "")
	if got := Redirected(); len(got) != 0 {
		t.Errorf("missing anchor: Redirected() = %v, want empty", got)
	}
	useAnchor(t, Render([]int{81}))
	if got := Redirected(); len(got) != 0 {
		t.Errorf("stale anchor: Redirected() = %v, want empty", got)
	}
}

// The grant start needs to re-load the anchor must not depend on lerd
// managing DNS, or a reboot leaves nginx on a port nothing redirects to.
func TestSudoersGrantsExactlyTheApplyCommand(t *testing.T) {
	got := Sudoers("alice")
	want := "alice ALL=(root) NOPASSWD: /sbin/pfctl -E -a com.apple/lerd -f /etc/pf.anchors/lerd\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("Sudoers() = %q, want it to end with %q", got, want)
	}
	if strings.Count(got, "NOPASSWD") != 1 {
		t.Errorf("Sudoers() must grant one command only: %q", got)
	}
}

// A reboot only drops the anchor, so the granted reload leads, and reloading
// pf.conf, which wipes a VPN's rules, is offered only for a skipped lo0.
func TestRepairLeadsWithTheGrantedApply(t *testing.T) {
	got := Repair()
	if !strings.HasPrefix(got, "sudo /sbin/pfctl -E -a com.apple/lerd -f /etc/pf.anchors/lerd,") {
		t.Errorf("Repair() = %q, want it to lead with the granted anchor reload", got)
	}
	if !strings.Contains(got, "lo0 (skip), sudo /sbin/pfctl -f /etc/pf.conf before it") {
		t.Errorf("Repair() = %q, want the pf.conf reload only on a skipped lo0", got)
	}
	if !strings.HasSuffix(ErrNotEffective.Error(), got) {
		t.Errorf("ErrNotEffective = %q, want the same repair as doctor", ErrNotEffective)
	}
}

func TestEffectiveChecksEveryRedirectedPort(t *testing.T) {
	prev := reaches
	t.Cleanup(func() { reaches = prev })
	var probed []int
	reaches = func(from, to int) bool {
		probed = append(probed, from)
		return to == Target(from) && from != 443
	}
	useAnchor(t, "")
	if !Effective() || len(probed) != 0 {
		t.Errorf("without an anchor there is nothing to check, probed %v", probed)
	}
	useAnchor(t, Render([]int{80, 443}))
	if Effective() {
		t.Error("Effective() = true with 443 bypassing the redirect")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func listenOn(t *testing.T, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", loopbackAddr(port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
}

// Without pf in a test, a port that is its own target stands in for a working
// redirect, and a listener on the source port for gvproxy's wide bind.
func TestLoopbackReachesOnlyWhenTheConnectionArrivesOnTheTarget(t *testing.T) {
	from, to := freePort(t), freePort(t)
	if !loopbackReaches(to, to) {
		t.Error("a connection that arrives on the target must count")
	}
	if loopbackReaches(from, to) {
		t.Error("nothing listens on the source port, so nothing was redirected")
	}
	listenOn(t, from)
	if loopbackReaches(from, to) {
		t.Error("an answer from a wide bind on the source port must not pass for the redirect")
	}
}

func TestLoopbackReachesWithTheTargetHeldTrustsAnAnswer(t *testing.T) {
	from, to := freePort(t), freePort(t)
	listenOn(t, to)
	if loopbackReaches(from, to) {
		t.Error("nothing answers on the source port")
	}
	listenOn(t, from)
	if !loopbackReaches(from, to) {
		t.Error("with nginx holding the target, an answer on the source port counts")
	}
}

func TestAcceptsOwnSkipsAConnectionThatArrivedFirst(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	other, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	own, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	if !acceptsOwn(ln.(*net.TCPListener), own.LocalAddr().String(), time.Now().Add(probeTimeout)) {
		t.Error("a browser's connection landing first must not hide the probe's own")
	}
	if acceptsOwn(ln.(*net.TCPListener), "127.0.0.1:1", time.Now().Add(100*time.Millisecond)) {
		t.Error("without the probe's own connection the check must fail at the deadline")
	}
}
