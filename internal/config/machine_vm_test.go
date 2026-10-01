package config

import "testing"

func TestUsesMachineVM(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "windows": true, "linux": false, "freebsd": false} {
		if got := usesMachineVM(goos); got != want {
			t.Errorf("usesMachineVM(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestControlChannelFollowsThePlatform(t *testing.T) {
	if n, a := controlEndpoint("windows", "/run/x.sock"); n != "udp" || a != "127.0.0.1:"+ControlUDPPort {
		t.Errorf("windows control = %s %s, want loopback udp", n, a)
	}
	if n, a := controlEndpoint("darwin", "/run/x.sock"); n != "unixgram" || a != "/run/x.sock" {
		t.Errorf("darwin control = %s %s, want the unix datagram socket", n, a)
	}
	if n, a := controlEndpoint("linux", "/run/x.sock"); n != "unixgram" || a != "/run/x.sock" {
		t.Errorf("linux control = %s %s", n, a)
	}
}
