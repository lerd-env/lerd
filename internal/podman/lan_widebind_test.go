package podman

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/pfredirect"
	"github.com/geodro/lerd/internal/platform"
)

func widePrivilegedPorts(t *testing.T, on bool) {
	t.Helper()
	prev := platform.Current
	platform.Current.PrivilegedPortsBindAll = on
	t.Cleanup(func() { platform.Current = prev })
	useAnchor(t, filepath.Join(t.TempDir(), "missing"))
}

// useAnchor points the pf redirect at path, so the host's real anchor never
// leaks into a test.
func useAnchor(t *testing.T, path string) {
	t.Helper()
	prev := pfredirect.AnchorPath
	pfredirect.AnchorPath = path
	t.Cleanup(func() { pfredirect.AnchorPath = prev })
}

// On macOS a loopback publish below 1024 loses its address, so lan:status said
// loopback-only while nginx answered the whole network on 80 and 443.
func TestLoopbackPortsPublishedWideNamesPrivilegedLoopbackBinds(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	widePrivilegedPorts(t, true)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=127.0.0.1:80:80\nPublishPort=[::1]:80:80\nPublishPort=127.0.0.1:443:443\nPublishPort=[::1]:443:443")
	writeLANQuadlet(t, "lerd-mysql", true, "PublishPort=127.0.0.1:3306:3306\nPublishPort=[::1]:3306:3306")
	writeLANQuadlet(t, "lerd-dns", false, "PublishPort=127.0.0.1:5300:5300")

	wide, err := LoopbackPortsPublishedWide()
	if err != nil {
		t.Fatalf("LoopbackPortsPublishedWide: %v", err)
	}
	if got := slices.Sorted(maps.Keys(wide)); !slices.Equal(got, []string{"lerd-nginx"}) {
		t.Fatalf("units = %v, want [lerd-nginx]", got)
	}
	if !slices.Equal(wide["lerd-nginx"], []int{80, 443}) {
		t.Errorf("lerd-nginx ports = %v, want [80 443]", wide["lerd-nginx"])
	}
}

func TestLoopbackPortsPublishedWideIgnoresBindsAlreadyOnTheLAN(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	widePrivilegedPorts(t, true)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=80:80\nPublishPort=[::]:443:443")

	wide, err := LoopbackPortsPublishedWide()
	if err != nil {
		t.Fatalf("LoopbackPortsPublishedWide: %v", err)
	}
	if len(wide) != 0 {
		t.Errorf("an exposed unit is not a loopback bind that widened, got %v", wide)
	}
}

func TestLoopbackPortsPublishedWideEmptyWhereLoopbackHolds(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	widePrivilegedPorts(t, false)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=127.0.0.1:80:80\nPublishPort=[::1]:80:80")

	wide, err := LoopbackPortsPublishedWide()
	if err != nil {
		t.Fatalf("LoopbackPortsPublishedWide: %v", err)
	}
	if len(wide) != 0 {
		t.Errorf("a platform that keeps loopback binds must report nothing, got %v", wide)
	}
}

func TestLoopbackPortsPublishedWideSkipsRedirectedPorts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	widePrivilegedPorts(t, true)
	anchor := filepath.Join(t.TempDir(), "lerd")
	if err := os.WriteFile(anchor, []byte(pfredirect.Render([]int{80, 443})), 0644); err != nil {
		t.Fatal(err)
	}
	useAnchor(t, anchor)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=127.0.0.1:80:80\nPublishPort=127.0.0.1:443:443")

	wide, err := LoopbackPortsPublishedWide()
	if err != nil {
		t.Fatalf("LoopbackPortsPublishedWide: %v", err)
	}
	if len(wide) != 0 {
		t.Errorf("ports pf keeps on loopback are not wide, got %v", wide)
	}
}

// The dashboard asks about lerd-nginx alone on every vhost request, so the
// lookup for one unit must not depend on, or read, any other unit.
func TestUnitLoopbackPortsPublishedWideReadsOnlyThatUnit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	widePrivilegedPorts(t, true)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=127.0.0.1:80:80\nPublishPort=127.0.0.1:443:443")
	writeLANQuadlet(t, "lerd-custom", false, "PublishPort=127.0.0.1:25:25")

	ports, err := UnitLoopbackPortsPublishedWide("lerd-nginx")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ports, []int{80, 443}) {
		t.Errorf("lerd-nginx ports = %v, want [80 443]", ports)
	}
	if ports, err := UnitLoopbackPortsPublishedWide("lerd-missing"); err != nil || len(ports) != 0 {
		t.Errorf("a unit that is not installed publishes nothing, got %v, %v", ports, err)
	}
}
