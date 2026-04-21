package podman

import (
	"net"
	"strings"
	"testing"
)

func TestLerdULAv6Subnet_isValidIPv6CIDR(t *testing.T) {
	ip, ipnet, err := net.ParseCIDR(LerdULAv6Subnet)
	if err != nil {
		t.Fatalf("LerdULAv6Subnet not parseable: %v", err)
	}
	if ip.To4() != nil {
		t.Errorf("LerdULAv6Subnet must be v6, got v4 %v", ip)
	}
	if ones, bits := ipnet.Mask.Size(); ones != 64 || bits != 128 {
		t.Errorf("expected /64 mask, got /%d (bits=%d)", ones, bits)
	}
	if !strings.HasPrefix(ip.String(), "fd") {
		t.Errorf("expected ULA prefix (fc00::/7), got %v", ip)
	}
}

func TestErrNetworkNeedsMigration_isComparable(t *testing.T) {
	if ErrNetworkNeedsMigration == nil {
		t.Fatal("ErrNetworkNeedsMigration is nil")
	}
	if ErrNetworkNeedsMigration.Error() == "" {
		t.Error("ErrNetworkNeedsMigration has empty message")
	}
}
