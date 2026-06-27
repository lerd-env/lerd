package dns

import "testing"

// Regression for the macOS false "lerd-dns container not running" warning:
// defaultContainerRunning must treat the lerd-dns service unit as running
// when the platform service manager reports it active (launchd on macOS,
// systemd on linux), instead of only checking systemctl + podman ps.
func TestDefaultContainerRunning_serviceActiveShortCircuits(t *testing.T) {
	prev := serviceActive
	t.Cleanup(func() { serviceActive = prev })

	serviceActive = func(name string) bool { return name == "lerd-dns" }
	if !defaultContainerRunning() {
		t.Error("expected true when the service manager reports lerd-dns active")
	}
}
