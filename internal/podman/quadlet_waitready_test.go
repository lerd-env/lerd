package podman

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestMySQLReadinessProbeForcesTCP(t *testing.T) {
	// The probe runs inside the container. With no host, mysqladmin falls back
	// to the Unix socket, whose path differs between the mysql and mariadb
	// images, so a socket probe times out even when the server is up.
	joined := strings.Join(mysqlReadyArgs, " ")

	if mysqlReadyArgs[0] != "mysqladmin" || mysqlReadyArgs[1] != "ping" {
		t.Fatalf("mysql readiness probe should be a mysqladmin ping, got: %s", joined)
	}
	if !strings.Contains(joined, "-h127.0.0.1") {
		t.Errorf("mysql readiness probe must force TCP via -h127.0.0.1, got: %s", joined)
	}
	if strings.Contains(joined, "localhost") {
		t.Errorf("mysql readiness probe must not use localhost — mysqladmin resolves it to the Unix socket, got: %s", joined)
	}
}

func TestMariaDBReadinessProbeUsesMariaDBAdmin(t *testing.T) {
	// mariadb:11 dropped the mysqladmin symlink, so the probe must call
	// mariadb-admin or WaitReady times out on every poll (issue #478).
	joined := strings.Join(mariadbReadyArgs, " ")

	if mariadbReadyArgs[0] != "mariadb-admin" || mariadbReadyArgs[1] != "ping" {
		t.Fatalf("mariadb readiness probe should be a mariadb-admin ping, got: %s", joined)
	}
	if strings.Contains(joined, "mysqladmin") {
		t.Errorf("mariadb readiness probe must not call mysqladmin — absent in mariadb:11, got: %s", joined)
	}
	if !strings.Contains(joined, "-h127.0.0.1") {
		t.Errorf("mariadb readiness probe must force TCP via -h127.0.0.1, got: %s", joined)
	}
}

func TestRustFSProbeAddrFollowsPublishedPort(t *testing.T) {
	// rustfs is the one service probed by a host-side TCP dial rather than an
	// in-container exec, so its probe must target the port rustfs is actually
	// published on, read from the registry's effective host port. When the
	// port-ownership guard or `lerd service port` moves rustfs off 9000 (a host
	// server owns it), a dial to a hardcoded 9000 never connects and WaitReady
	// burns its full timeout on every php/composer call.
	if got := rustfsProbeAddr(config.ServiceConfig{Port: 9000}); got != "localhost:9000" {
		t.Errorf("preset default: rustfsProbeAddr = %q, want localhost:9000", got)
	}
	if got := rustfsProbeAddr(config.ServiceConfig{Port: 9000, PublishedPort: 9002}); got != "localhost:9002" {
		t.Errorf("moved published port: rustfsProbeAddr = %q, want localhost:9002", got)
	}
	if got := rustfsProbeAddr(config.ServiceConfig{}); got != "" {
		t.Errorf("unconfigured: rustfsProbeAddr = %q, want empty (nothing to dial)", got)
	}
}

func TestReadyFamilyRoutesVersionedNames(t *testing.T) {
	cases := map[string]string{
		"mariadb":       "mariadb",
		"mariadb-11":    "mariadb",
		"mariadb-10-11": "mariadb",
		"mysql":         "mysql",
		"mysql-8-0":     "mysql",
		"postgres":      "postgres",
		"postgres-16":   "postgres",
		"redis":         "redis",
		"rustfs":        "rustfs",
		"custom-thing":  "custom-thing",
	}
	for service, want := range cases {
		if got := readyFamily(service); got != want {
			t.Errorf("readyFamily(%q) = %q, want %q", service, got, want)
		}
	}
}
