//go:build windows

package cli

import (
	"strings"
	"testing"
)

const netstatSample = `
Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:80             0.0.0.0:0              LISTENING       4
  TCP    0.0.0.0:8080           0.0.0.0:0              LISTENING       812
  TCP    [::]:3306              [::]:0                 LISTENING       990
  TCP    127.0.0.1:5432         127.0.0.1:50122        ESTABLISHED     990
`

func TestListeningOnPort(t *testing.T) {
	for port, want := range map[string]bool{
		"80": true, "8080": true, "3306": true, "443": false, "5432": false, "8": false,
	} {
		if got := listeningOnPort(netstatSample, port); got != want {
			t.Errorf("listeningOnPort(%s) = %v, want %v", port, got, want)
		}
	}
}

func TestListeningOnlyDropsEstablishedConnections(t *testing.T) {
	got := listeningOnly(netstatSample + "  TCP    192.168.1.8:50370      4.207.247.139:443      ESTABLISHED     4800\n")
	if strings.Contains(got, "ESTABLISHED") || strings.Contains(got, "4.207.247.139:443") {
		t.Errorf("established connections must not survive:\n%s", got)
	}
	for _, want := range []string{"0.0.0.0:80 ", "0.0.0.0:8080", "[::]:3306"} {
		if !strings.Contains(got, want) {
			t.Errorf("listener %q was dropped:\n%s", want, got)
		}
	}
	// A caller that greps for ":443" must not find a remote endpoint.
	if strings.Contains(got, ":443") {
		t.Errorf("a substring search for :443 still matches:\n%s", got)
	}
}
