//go:build linux

package unitlog

import "testing"

// lerd-dns runs as a host process, so its logs live in the journal and a
// podman lookup only ever reports a container that does not exist.
func TestIsContainerUnit_lerdDNSIsAHostService(t *testing.T) {
	if IsContainerUnit("lerd-dns") {
		t.Error("lerd-dns must read from the journal, not podman")
	}
	if !IsContainerUnit("lerd-nginx") {
		t.Error("lerd-nginx is still a container")
	}
}
