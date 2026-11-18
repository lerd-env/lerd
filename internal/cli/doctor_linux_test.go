//go:build linux

package cli

import "testing"

func TestPortHolder(t *testing.T) {
	// ss -tlnp only lists the process for sockets the caller owns, so a root
	// listener arrives with an empty process column.
	userOwned := `State  Recv-Q Send-Q Local Address:Port Peer Address:PortProcess
LISTEN 0      511          0.0.0.0:80        0.0.0.0:*    users:(("caddy",pid=4242,fd=7))
LISTEN 0      511          0.0.0.0:8080      0.0.0.0:*    users:(("node",pid=99,fd=3))
`
	rootOwned := `State  Recv-Q Send-Q Local Address:Port Peer Address:PortProcess
LISTEN 0      511          0.0.0.0:80        0.0.0.0:*
LISTEN 0      511             [::]:443          [::]:*
`
	none := func(string) bool { return false }
	only := func(unit string) func(string) bool {
		return func(u string) bool { return u == unit }
	}

	cases := []struct {
		name       string
		port, ss   string
		unitActive func(string) bool
		want       string
	}{
		{"user-owned process is named from ss", "80", userOwned, only("nginx"), "caddy (pid 4242)"},
		{"another port's process is not borrowed", "443", userOwned, none, ""},
		{"root listener falls back to a running web server unit", "80", rootOwned, only("httpd"), "the system httpd service"},
		{"root listener on 443 checks units too", "443", rootOwned, only("apache2"), "the system apache2 service"},
		{"root listener with no known unit is unknown", "80", rootOwned, none, ""},
		{"free port names nothing", "80", "", only("nginx"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := portHolder(c.port, c.ss, c.unitActive); got != c.want {
				t.Fatalf("portHolder(%s) = %q, want %q", c.port, got, c.want)
			}
		})
	}
}
