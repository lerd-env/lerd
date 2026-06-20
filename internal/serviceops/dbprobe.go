package serviceops

import (
	"net"
	"os"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// probeTimeout bounds each socket/TCP dial so the host-MySQL probe stays snappy
// for the dashboard even when nothing is listening.
const probeTimeout = 750 * time.Millisecond

// legacyMySQLSocket is the older /var/run path some distros still symlink the
// MySQL socket to; probed as a fallback when the default /run path is absent.
const legacyMySQLSocket = "/var/run/mysqld/mysqld.sock"

// HostMySQLStatus reports whether a host-installed (system) MySQL/MariaDB is
// present and reachable, for the dashboard's "system MySQL" backend option.
// It needs no MySQL client — only a socket stat, a unix dial, and a TCP probe.
type HostMySQLStatus struct {
	SocketPresent    bool   `json:"socket_present"`
	SocketPath       string `json:"socket_path"`
	Live             bool   `json:"live"`              // a server is accepting on the socket
	TCP3306Listening bool   `json:"tcp3306_listening"` // something answers on 127.0.0.1:3306
	TCP3306Owner     string `json:"tcp3306_owner"`     // host | lerd | none | unknown
}

// ProbeHostMySQL inspects the host MySQL unix socket at socketPath (empty =>
// config.DefaultHostMySQLSocket, with the /var/run fallback) and 127.0.0.1:3306,
// then attributes the TCP listener. lerd's rootless MySQL container never
// exposes a host unix socket, so socket presence is the strong "this is the
// host server" discriminator.
func ProbeHostMySQL(socketPath string) HostMySQLStatus {
	path := socketPath
	if path == "" {
		path = config.DefaultHostMySQLSocket
	}
	present, live := socketLive(path, probeTimeout)
	if !present && path != legacyMySQLSocket {
		if p2, l2 := socketLive(legacyMySQLSocket, probeTimeout); p2 {
			path, present, live = legacyMySQLSocket, p2, l2
		}
	}
	listening := tcpListening("127.0.0.1:3306", probeTimeout)
	return HostMySQLStatus{
		SocketPresent:    present,
		SocketPath:       path,
		Live:             live,
		TCP3306Listening: listening,
		TCP3306Owner:     attributeTCP3306Owner(listening, present, podman.ContainerRunningQuiet("lerd-mysql")),
	}
}

// socketLive reports whether path is a unix socket (present) and whether a
// server is currently accepting connections on it (live). A stale socket file
// left by a crashed server is present but not live.
func socketLive(path string, timeout time.Duration) (present, live bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		return false, false
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return true, false
	}
	_ = conn.Close()
	return true, true
}

// tcpListening reports whether something is accepting TCP connections at addr.
func tcpListening(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// attributeTCP3306Owner decides who owns 127.0.0.1:3306 from three signals.
// A running lerd-mysql container publishes the port, so it wins attribution;
// otherwise a present host socket means the host server owns it. The user runs
// one or the other on 3306 (they conflict), so this matches reality.
func attributeTCP3306Owner(listening, socketPresent, lerdRunning bool) string {
	switch {
	case !listening:
		return "none"
	case lerdRunning:
		return "lerd"
	case socketPresent:
		return "host"
	default:
		return "unknown"
	}
}
