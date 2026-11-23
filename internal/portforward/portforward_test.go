package portforward

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/pfredirect"
)

// isolate points the plist dir and the legacy pf anchor at temp paths.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	prevDir, prevAnchor := PlistDir, pfredirect.AnchorPath
	PlistDir = t.TempDir()
	pfredirect.AnchorPath = filepath.Join(t.TempDir(), "lerd")
	t.Cleanup(func() { PlistDir, pfredirect.AnchorPath = prevDir, prevAnchor })
}

// launchd binds the port as root, but the forwarder it starts must run as the
// user, or a user-writable binary would run as root.
func TestRenderRunsTheForwarderAsTheUserWithTheSocketOnStdin(t *testing.T) {
	got := Render(80, "alice", "/Users/alice/.local/bin/lerd")
	for _, want := range []string{
		"<key>Label</key>\n\t<string>com.lerd.port-forward-80</string>",
		"<string>/Users/alice/.local/bin/lerd</string>\n\t\t<string>port-forward</string>\n\t\t<string>--to</string>\n\t\t<string>17080</string>",
		"<key>UserName</key>\n\t<string>alice</string>",
		"<key>Wait</key>\n\t\t<true/>",
		"<key>SockServiceName</key>\n\t\t\t<string>80</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q:\n%s", want, got)
		}
	}
	// Every interface, so LAN exposure needs no root; the forwarder gates it.
	if strings.Contains(got, "SockNodeName") {
		t.Error("the job must listen on every interface")
	}
	if strings.Contains(Render(80, "a<b", "/x&y"), "a<b") {
		t.Error("values must be XML-escaped")
	}
}

func TestRedirectedPrefersTheForwarderOverTheLegacyAnchor(t *testing.T) {
	isolate(t)
	if len(Redirected()) != 0 {
		t.Fatal("nothing installed, nothing redirected")
	}
	if err := os.WriteFile(pfredirect.AnchorPath, []byte(pfredirect.Render([]int{80, 443})), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Redirected(); !got[80] || !got[443] {
		t.Errorf("until install migrates, the pf anchor still counts: %v", got)
	}
	os.Remove(pfredirect.AnchorPath)
	if err := os.WriteFile(PlistPath(80), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Redirected(); !got[80] || got[443] {
		t.Errorf("Redirected() = %v, want only the forwarded 80", got)
	}
}

func TestServeRelaysBothWays(t *testing.T) {
	up, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	go func() {
		c, err := up.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 4)
		io.ReadFull(c, buf)
		c.Write([]byte(strings.ToUpper(string(buf))))
	}()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go Serve(ln, up.Addr().(*net.TCPAddr).Port, func() bool { return false })

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "PING" {
		t.Errorf("relay returned %q, %v; want PING", buf, err)
	}
}

// Off the LAN must mean off: a connection that did not arrive on loopback is
// dropped unless LAN exposure is on, read fresh for every connection.
func TestAllowedLetsTheLANInOnlyWhenExposed(t *testing.T) {
	loop := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80}
	lan := &net.TCPAddr{IP: net.ParseIP("192.168.0.166"), Port: 80}
	exposed := false
	lanAllowed := func() bool { return exposed }
	if !allowed(loop, lanAllowed) {
		t.Error("loopback must always be served")
	}
	if allowed(lan, lanAllowed) {
		t.Error("the LAN must be dropped while unexposed")
	}
	exposed = true
	if !allowed(lan, lanAllowed) {
		t.Error("the LAN must be served once exposed")
	}
}

func TestInstalledIgnoresTheLegacyAnchor(t *testing.T) {
	isolate(t)
	if err := os.WriteFile(pfredirect.AnchorPath, []byte(pfredirect.Render([]int{80, 443})), 0644); err != nil {
		t.Fatal(err)
	}
	if Installed() {
		t.Error("the pf rule does not cover the LAN, so it must not count")
	}
	if err := os.WriteFile(PlistPath(80), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !Installed() {
		t.Error("a launchd job should count")
	}
}
