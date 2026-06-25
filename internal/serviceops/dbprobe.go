package serviceops

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// probeTimeout bounds each socket/TCP dial so the host-DB probe stays snappy for
// the dashboard even when nothing is listening.
const probeTimeout = 750 * time.Millisecond

// HostDBStatus reports whether a host-installed (system) database server of a
// given family (MySQL/MariaDB/Postgres) is present and reachable, for the
// dashboard's "system database" backend option. It needs no DB client — only a
// socket stat, a unix dial, and a TCP probe.
type HostDBStatus struct {
	ServiceName   string `json:"service_name"`   // lerd service probed (mysql/postgres)
	Port          int    `json:"port"`           // the engine's canonical TCP port (3306/5432)
	SocketPresent bool   `json:"socket_present"` // a unix socket exists for SocketPath
	SocketPath    string `json:"socket_path"`    // socket file (MySQL) or socket dir (Postgres)
	Live          bool   `json:"live"`           // a server is accepting on the socket
	PortListening bool   `json:"port_listening"` // something answers on 127.0.0.1:Port
	PortOwner     string `json:"port_owner"`     // host | lerd | none | unknown
	// LerdPort is the host port lerd's own container publishes (the engine default,
	// or the configured PublishedPort override). When it differs from Port the host
	// server can own the default port and the two coexist. LerdPortListening reports
	// whether something answers on 127.0.0.1:<LerdPort>.
	LerdPort          int  `json:"lerd_port"`
	LerdPortListening bool `json:"lerd_port_listening"`
}

// ProbeHostMySQL is a thin back-compat wrapper around ProbeHostDB for the mysql
// service. Deprecated: prefer ProbeHostDB(serviceName, socketPath).
func ProbeHostMySQL(socketPath string) HostDBStatus {
	return ProbeHostDB("mysql", socketPath)
}

// ProbeHostDB inspects a host-installed database server for the given lerd
// service (e.g. "mysql", "postgres"). It checks the family's host unix socket
// (socketPath overrides the default; empty => the family default plus its distro
// fallbacks) and the engine's canonical TCP port, then attributes the port's
// listener. lerd's rootless DB containers never expose a host unix socket, so
// socket presence is the strong "this is the host server" discriminator.
func ProbeHostDB(serviceName, socketPath string) HostDBStatus {
	if serviceName == "" {
		serviceName = "mysql"
	}
	spec, ok := config.HostBackendFor(config.FamilyOfName(serviceName))
	if !ok {
		// Not a host-capable DB service: nothing meaningful to probe.
		return HostDBStatus{ServiceName: serviceName}
	}

	// Candidate socket paths: the explicit override, else the family default plus
	// its distro fallbacks. For Postgres these are DIRECTORIES; the actual unix
	// socket is <dir>/.s.PGSQL.<port> inside.
	var candidates []string
	if socketPath != "" {
		candidates = []string{socketPath}
	} else {
		candidates = append([]string{spec.HostSocketPath()}, spec.LinuxSocketFallbacks...)
	}
	socketFileFor := func(p string) string {
		if spec.SocketIsDir {
			return filepath.Join(p, fmt.Sprintf(".s.PGSQL.%d", spec.DefaultPort))
		}
		return p
	}
	chosen := candidates[0]
	var present, live bool
	for _, c := range candidates {
		if p, l := socketLive(socketFileFor(c), probeTimeout); p {
			chosen, present, live = c, p, l
			break
		}
	}

	listening := tcpListening(net.JoinHostPort("127.0.0.1", strconv.Itoa(spec.DefaultPort)), probeTimeout)
	lerdPort := lerdServicePort(serviceName, spec.DefaultPort)
	lerdListening := listening // when lerd is on the default port, its listener is that one
	if lerdPort != spec.DefaultPort {
		lerdListening = tcpListening(net.JoinHostPort("127.0.0.1", strconv.Itoa(lerdPort)), probeTimeout)
	}
	// lerd's container for a family alternate is lerd-<full service name> (e.g.
	// lerd-mariadb-11, lerd-postgres-pgvector), not the canonical spec.ContainerName —
	// match the codebase-wide "lerd-"+name convention so attribution sees the actual
	// running container. serviceName is never empty here (defaulted to "mysql" above),
	// so this yields lerd-mysql for the canonical case.
	lerdRunning := podman.ContainerRunningQuiet("lerd-" + serviceName)
	return HostDBStatus{
		ServiceName:       serviceName,
		Port:              spec.DefaultPort,
		SocketPresent:     present,
		SocketPath:        chosen,
		Live:              live,
		PortListening:     listening,
		PortOwner:         attributePortOwner(listening, present, lerdRunning, lerdPort == spec.DefaultPort),
		LerdPort:          lerdPort,
		LerdPortListening: lerdListening,
	}
}

// lerdServicePort returns the host port lerd's container for service publishes:
// the configured PublishedPort override when set, otherwise the engine default.
func lerdServicePort(service string, defaultPort int) int {
	if cfg, err := config.LoadGlobal(); err == nil {
		if sc, ok := cfg.Services[service]; ok && sc.PublishedPort > 0 {
			return sc.PublishedPort
		}
	}
	return defaultPort
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

// attributePortOwner decides who owns 127.0.0.1:<engine default port>.
// lerdOnDefaultPort says whether lerd's container publishes that default port
// (true unless it was moved off via PublishedPort). A running lerd container that
// still publishes the default port wins attribution (it would conflict with a
// host server there); once lerd is moved off the default port, a listener there
// is the host server, so a present socket — or just the listener itself —
// attributes to the host. With lerdOnDefaultPort=true the result is identical to
// the previous three-signal behaviour, so existing callers are unaffected.
func attributePortOwner(listening, socketPresent, lerdRunning, lerdOnDefaultPort bool) string {
	switch {
	case !listening:
		return "none"
	case lerdRunning && lerdOnDefaultPort:
		return "lerd"
	case socketPresent:
		return "host"
	case lerdRunning && !lerdOnDefaultPort:
		// lerd is running elsewhere, yet the default port has a listener: it must be the host.
		return "host"
	default:
		return "unknown"
	}
}
