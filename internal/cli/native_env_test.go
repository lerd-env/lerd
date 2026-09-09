package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// In container mode only a host-proxy site reaches services over loopback.
func TestUsesLoopbackServicesUnderContainerRuntime(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if usesLoopbackServices(&config.Site{}) {
		t.Error("a containerised site must keep container DNS names")
	}
	if !usesLoopbackServices(&config.Site{HostPort: 3000}) {
		t.Error("a host-proxy site always runs on the host")
	}
}

// The port map decides whether a rewritten DSN reaches lerd's service or
// whatever else owns the container's port on the host. Building it only from
// the services a .lerd.yaml happens to declare leaves a site without that file
// pointed at 127.0.0.1 on the *container* port, which on a machine running its
// own MySQL connects successfully to the wrong database.
func TestLoopbackServiceNamesCoverUndeclaredServices(t *testing.T) {
	declared := map[string]bool{"redis": true}
	detected := []string{"mysql", "redis"}

	names := loopbackServiceNames(declared, detected)

	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if !got["mysql"] {
		t.Errorf("a service used but not declared in .lerd.yaml must still be mapped, got %v", names)
	}
	if !got["redis"] {
		t.Errorf("a declared service must stay mapped, got %v", names)
	}
}

func TestLoopbackServiceNamesDeduplicates(t *testing.T) {
	names := loopbackServiceNames(map[string]bool{"mysql": true}, []string{"mysql", "mysql"})
	if len(names) != 1 {
		t.Errorf("expected one entry, got %v", names)
	}
}

// Some frameworks carry the port inside the host value (WordPress writes
// DB_HOST as host:port and has no DB_PORT constant). Rewriting a bare
// lerd-mysql to plain 127.0.0.1 drops the port, so the app silently tries
// 3306 and reaches whatever else owns it rather than lerd's MySQL.
func TestBareHostKeepsANonDefaultPort(t *testing.T) {
	containerToHost := map[string]string{"3306": "3307"}
	serviceContainerPort := map[string]string{"mysql": "3306"}

	updates := map[string]string{"DB_HOST": "lerd-mysql"}
	applyHostProxyEnvWithPorts(updates, containerToHost, serviceContainerPort)
	if got := updates["DB_HOST"]; got != "127.0.0.1:3307" {
		t.Errorf("DB_HOST = %q, want 127.0.0.1:3307", got)
	}
}

// When a sibling *_PORT carries the port, the host must stay bare or the value
// ends up with the port twice.
func TestBareHostStaysBareWhenASiblingPortExists(t *testing.T) {
	updates := map[string]string{"DB_HOST": "lerd-mysql", "DB_PORT": "3306"}
	applyHostProxyEnvWithPorts(updates, map[string]string{"3306": "3307"}, map[string]string{"mysql": "3306"})
	if got := updates["DB_HOST"]; got != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want a bare 127.0.0.1", got)
	}
	if got := updates["DB_PORT"]; got != "3307" {
		t.Errorf("DB_PORT = %q, want 3307", got)
	}
}

// A service published on its own port needs no suffix.
func TestBareHostStaysBareWhenPortsMatch(t *testing.T) {
	updates := map[string]string{"REDIS_HOST": "lerd-redis"}
	applyHostProxyEnvWithPorts(updates, map[string]string{"6379": "6379"}, map[string]string{"redis": "6379"})
	if got := updates["REDIS_HOST"]; got != "127.0.0.1" {
		t.Errorf("REDIS_HOST = %q, want a bare 127.0.0.1", got)
	}
}

// Frameworks that keep configuration in a PHP array use dotted keys
// (TYPO3 writes DB.Connections.Default.host, not DB_HOST). The connection-key
// guard only knew SCREAMING_SNAKE suffixes, so those frameworks kept pointing
// at lerd-mysql and broke the moment PHP moved to the host.
func TestConnKeyRecognisesDottedKeys(t *testing.T) {
	for _, k := range []string{
		"DB.Connections.Default.host",
		"DB.Connections.Default.port",
		"db.connection.default.host",
	} {
		if !hostProxyConnKey(k) {
			t.Errorf("%q should be treated as a connection key", k)
		}
	}
}

// The guard still has to keep its original job: a value that merely contains a
// lerd- token must not be rewritten into loopback.
func TestConnKeyStillIgnoresNonConnectionKeys(t *testing.T) {
	for _, k := range []string{"APP_NAME", "MIX_APP_NAME", "DB.Connections.Default.charset", "SOME.random.label"} {
		if hostProxyConnKey(k) {
			t.Errorf("%q should not be treated as a connection key", k)
		}
	}
}

func TestDottedHostAndPortAreRewritten(t *testing.T) {
	updates := map[string]string{
		"DB.Connections.Default.host":    "lerd-mysql",
		"DB.Connections.Default.port":    "3306",
		"DB.Connections.Default.charset": "utf8mb4",
	}
	applyHostProxyEnvWithPorts(updates, map[string]string{"3306": "3307"}, map[string]string{"mysql": "3306"})
	if got := updates["DB.Connections.Default.host"]; got != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", got)
	}
	if got := updates["DB.Connections.Default.port"]; got != "3307" {
		t.Errorf("port = %q, want 3307", got)
	}
	if got := updates["DB.Connections.Default.charset"]; got != "utf8mb4" {
		t.Errorf("charset must be untouched, got %q", got)
	}
}
