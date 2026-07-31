package podman

import (
	"slices"
	"testing"
)

// stubRuntimeBinds points the runtime probe at a fixed answer per container.
func stubRuntimeBinds(t *testing.T, lanBound map[string]bool) {
	t.Helper()
	prev := ContainerPublishesLANFn
	ContainerPublishesLANFn = func(name string) (bool, bool) {
		bound, known := lanBound[name]
		return bound, known
	}
	t.Cleanup(func() { ContainerPublishesLANFn = prev })
}

// A container left LAN-bound by a toggle that aborted part way is the whole
// point of the runtime probe: the quadlet on disk already says loopback, so
// file comparison alone reports nothing to do and the drift never heals.
func TestRebindReportsRuntimeDriftWhenFileAlreadyCorrect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, false, false)
	writeLANQuadlet(t, "lerd-redis", true, "PublishPort=127.0.0.1:6379:6379\nPublishPort=[::1]:6379:6379")
	stubRuntimeBinds(t, map[string]bool{"lerd-redis": true})

	restart, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	if !slices.Contains(restart, "lerd-redis") {
		t.Fatalf("restart list %v omits the container still bound to the LAN", restart)
	}
}

// The mirror case: policy says LAN, the file says LAN, but the container is
// still running its old loopback bind.
func TestRebindReportsRuntimeDriftWhenContainerStillLoopback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, true, true)
	writeLANQuadlet(t, "lerd-mysql", true, "PublishPort=[::]:3306:3306")
	stubRuntimeBinds(t, map[string]bool{"lerd-mysql": false})

	restart, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	if !slices.Contains(restart, "lerd-mysql") {
		t.Fatalf("restart list %v omits the container still bound to loopback", restart)
	}
}

// A container whose runtime already matches the policy needs no restart, so a
// repeated toggle stays quiet.
func TestRebindStaysQuietWhenRuntimeMatches(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, false, false)
	writeLANQuadlet(t, "lerd-redis", true, "PublishPort=127.0.0.1:6379:6379\nPublishPort=[::1]:6379:6379")
	stubRuntimeBinds(t, map[string]bool{"lerd-redis": false})

	restart, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	if len(restart) != 0 {
		t.Fatalf("restart list = %v, want empty", restart)
	}
}

// Nothing running (or no podman at all) must not invent restarts.
func TestRebindIgnoresUnknownRuntimeState(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, false, false)
	writeLANQuadlet(t, "lerd-redis", true, "PublishPort=127.0.0.1:6379:6379\nPublishPort=[::1]:6379:6379")
	stubRuntimeBinds(t, map[string]bool{})

	restart, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	if len(restart) != 0 {
		t.Fatalf("restart list = %v, want empty", restart)
	}
}

func TestPortsPublishToLAN(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ports string
		want  bool
	}{
		{"dual-stack loopback", "127.0.0.1:3306->3306/tcp, ::1:3306->3306/tcp, 33060/tcp", false},
		{"ipv6 wildcard", ":::3306->3306/tcp, 33060/tcp", true},
		{"ipv4 wildcard", "0.0.0.0:80->80/tcp", true},
		{"mixed", "127.0.0.1:1025->1025/tcp, :::8025->8025/tcp", true},
		{"unpublished only", "9000/tcp", false},
		{"empty", "", false},
		{"udp loopback", "127.0.0.1:5300->5300/udp", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := portsPublishToLAN(tc.ports); got != tc.want {
				t.Errorf("portsPublishToLAN(%q) = %v, want %v", tc.ports, got, tc.want)
			}
		})
	}
}
