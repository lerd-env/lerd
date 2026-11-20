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
		{"root listener falls back to a running web server unit", "80", rootOwned, only("httpd"), "another process, possibly the system httpd service"},
		{"root listener on 443 checks units too", "443", rootOwned, only("apache2"), "another process, possibly the system apache2 service"},
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

func TestOtherUserLerdUID(t *testing.T) {
	// ss -tlne shows every socket's owner uid and cgroup, even another user's.
	ss := `State  Recv-Q Send-Q Local Address:Port Peer Address:PortProcess
LISTEN 0      4096       127.0.0.1:80         0.0.0.0:*    uid:1000 ino:140913 sk:3004 cgroup:/user.slice/user-1000.slice/user@1000.service/app.slice/lerd-nginx.service <->
LISTEN 0      4096               *:7073             *:*    uid:1000 ino:144602 sk:2d cgroup:/user.slice/user-1000.slice/user@1000.service/app.slice/lerd-ui.service v6only:0 <->
LISTEN 0      511          0.0.0.0:8080      0.0.0.0:*    uid:1000 ino:1 sk:1 cgroup:/user.slice/user-1000.slice/user@1000.service/app.slice/node.service <->
LISTEN 0      511          0.0.0.0:443       0.0.0.0:*    ino:2 sk:2 cgroup:/system.slice/nginx.service <->
`
	cases := []struct {
		name    string
		ports   []string
		selfUID string
		want    string
	}{
		{"another user's lerd-nginx holds 80", []string{"80", "443"}, "1001", "1000"},
		{"another user's lerd-ui holds the dashboard port", []string{"7073"}, "1001", "1000"},
		{"our own lerd is not another user", []string{"80", "7073"}, "1000", ""},
		{"another user's non-lerd process is not lerd", []string{"8080"}, "1001", ""},
		{"a root system nginx is not lerd", []string{"443"}, "1001", ""},
		{"a free port", []string{"5300"}, "1001", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := otherUserLerdUID(ss, c.ports, c.selfUID); got != c.want {
				t.Errorf("otherUserLerdUID(%v, self %s) = %q, want %q", c.ports, c.selfUID, got, c.want)
			}
		})
	}
}
