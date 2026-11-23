// Package portforward keeps nginx's privileged ports on loopback on macOS
// without pf, which turns iCloud Private Relay off while loaded (#2634). A
// root-owned launchd job holds 127.0.0.1:<port> and starts `lerd port-forward`
// as the user, handing it the listening socket, which relays to nginx's port.
package portforward

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/pfredirect"
)

// PlistDir is a var so tests can point it at a fixture.
var PlistDir = "/Library/LaunchDaemons"

// Target is the loopback port nginx publishes a forwarded port on.
func Target(port int) int { return pfredirect.Target(port) }

// Label is the launchd job holding port.
func Label(port int) string { return fmt.Sprintf("com.lerd.port-forward-%d", port) }

// PlistPath is where the job holding port is defined.
func PlistPath(port int) string { return filepath.Join(PlistDir, Label(port)+".plist") }

// Render returns the job holding port. Wait mode hands the forwarder the
// listening socket on stdin, so one process serves every connection.
func Render(port int, user, bin string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>port-forward</string>
		<string>--to</string>
		<string>%d</string>
	</array>
	<key>UserName</key>
	<string>%s</string>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>inetdCompatibility</key>
	<dict>
		<key>Wait</key>
		<true/>
	</dict>
	<key>Sockets</key>
	<dict>
		<key>Listeners</key>
		<dict>
			<key>SockNodeName</key>
			<string>127.0.0.1</string>
			<key>SockServiceName</key>
			<string>%d</string>
			<key>SockFamily</key>
			<string>IPv4</string>
			<key>SockType</key>
			<string>stream</string>
		</dict>
	</dict>
</dict>
</plist>
`, Label(port), escape(bin), Target(port), escape(user), port)
}

func escape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s)) //nolint:errcheck // writes to a buffer
	return b.String()
}

// Redirected returns nginx's ports a launchd job holds, or the legacy pf
// anchor's until lerd install replaces it, so nginx keeps its loopback publish.
func Redirected() map[int]bool {
	set := map[int]bool{}
	for _, p := range pfredirect.Ports() {
		if _, err := os.Stat(PlistPath(p)); err == nil {
			set[p] = true
		}
	}
	if len(set) > 0 {
		return set
	}
	return pfredirect.Redirected()
}

// Reaches reports whether a loopback connection to port arrives on its Target.
func Reaches(port int) bool { return pfredirect.Reaches(port) }

// reaches is a var so tests can stand in for launchd.
var reaches = Reaches

// Effective reports whether loopback connections to every redirected port
// arrive on its target.
func Effective() bool {
	for p := range Redirected() {
		if !reaches(p) {
			return false
		}
	}
	return true
}

// Serve relays every connection ln accepts to 127.0.0.1:to.
func Serve(ln net.Listener, to int) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go relay(c, fmt.Sprintf("127.0.0.1:%d", to))
	}
}

func relay(c net.Conn, to string) {
	defer c.Close()
	up, err := net.DialTimeout("tcp", to, 5*time.Second)
	if err != nil {
		return
	}
	defer up.Close()
	go func() {
		io.Copy(up, c) //nolint:errcheck
		up.(*net.TCPConn).CloseWrite()
	}()
	io.Copy(c, up) //nolint:errcheck
}
