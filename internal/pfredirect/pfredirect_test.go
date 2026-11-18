package pfredirect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
