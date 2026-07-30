package podman

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestRebindInstalledQuadletsForLANKeepsServicesPrivateByDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, true, false)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=127.0.0.1:443:443\nPublishPort=[::1]:443:443")
	writeLANQuadlet(t, "lerd-redis", true, "PublishPort=127.0.0.1:6379:6379\nPublishPort=[::1]:6379:6379")

	changed, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	if !slices.Equal(changed, []string{"lerd-nginx"}) {
		t.Fatalf("changed units = %v, want [lerd-nginx]", changed)
	}
	if content := readLANQuadlet(t, "lerd-nginx"); strings.Contains(content, "127.0.0.1:") || strings.Contains(content, "[::1]:") {
		t.Fatalf("nginx remains loopback-bound:\n%s", content)
	}
	if content := readLANQuadlet(t, "lerd-redis"); !strings.Contains(content, "PublishPort=127.0.0.1:6379:6379") {
		t.Fatalf("redis did not remain loopback-bound:\n%s", content)
	}
}

func TestRebindInstalledQuadletsForLANExposesOnlyOptedInServices(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, true, true)
	writeLANQuadlet(t, "lerd-nginx", false, "PublishPort=127.0.0.1:443:443\nPublishPort=[::1]:443:443")
	writeLANQuadlet(t, "lerd-mysql", true, "PublishPort=127.0.0.1:3306:3306\nPublishPort=[::1]:3306:3306")
	writeLANQuadlet(t, "lerd-custom-search", true, "PublishPort=127.0.0.1:7700:7700\nPublishPort=[::1]:7700:7700")
	writeLANQuadlet(t, "lerd-site-worker", false, "PublishPort=127.0.0.1:9000:9000\nPublishPort=[::1]:9000:9000")
	writeLANQuadlet(t, "lerd-dns", false, "PublishPort=127.0.0.1:5300:5300\nPublishPort=[::1]:5300:5300")

	changed, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	for _, name := range []string{"lerd-nginx", "lerd-mysql", "lerd-custom-search"} {
		if !slices.Contains(changed, name) {
			t.Errorf("changed units %v do not include %s", changed, name)
		}
		content := readLANQuadlet(t, name)
		if strings.Contains(content, "127.0.0.1:") || strings.Contains(content, "[::1]:") {
			t.Errorf("%s remains loopback-bound:\n%s", name, content)
		}
	}
	for _, name := range []string{"lerd-site-worker", "lerd-dns"} {
		if slices.Contains(changed, name) {
			t.Errorf("%s must remain loopback-bound, changed units: %v", name, changed)
		}
	}
}

func TestRebindInstalledQuadletsForLANRestoresLoopbackAndIsIdempotent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, false, true)
	writeLANQuadlet(t, "lerd-redis", true, "PublishPort=[::]:6379:6379")

	changed, err := RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("RebindInstalledQuadletsForLAN: %v", err)
	}
	if !slices.Equal(changed, []string{"lerd-redis"}) {
		t.Fatalf("changed units = %v, want [lerd-redis]", changed)
	}
	content := readLANQuadlet(t, "lerd-redis")
	if !strings.Contains(content, "PublishPort=127.0.0.1:6379:6379") || !strings.Contains(content, "PublishPort=[::1]:6379:6379") {
		t.Fatalf("redis was not restored to dual-stack loopback:\n%s", content)
	}

	changed, err = RebindInstalledQuadletsForLAN()
	if err != nil {
		t.Fatalf("second RebindInstalledQuadletsForLAN: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("idempotent rebind changed %v", changed)
	}
}

func TestWriteQuadletDiffAppliesServiceExposureToNewServices(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLANConfig(t, true, true)
	content := CustomServiceQuadletMarker + "\n[Container]\nImage=docker.io/library/redis:7.4.9-alpine\nNetwork=lerd\nPublishPort=127.0.0.1:6379:6379\n"

	if _, err := WriteQuadletDiff("lerd-redis", content); err != nil {
		t.Fatalf("WriteQuadletDiff: %v", err)
	}
	written := readLANQuadlet(t, "lerd-redis")
	if strings.Contains(written, "127.0.0.1:") || strings.Contains(written, "[::1]:") {
		t.Fatalf("new service did not inherit opted-in LAN exposure:\n%s", written)
	}
}

func writeLANConfig(t *testing.T, exposed, servicesExposed bool) {
	t.Helper()
	cfg := &config.GlobalConfig{}
	cfg.LAN.Exposed = exposed
	cfg.LAN.ServicesExposed = servicesExposed
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

func writeLANQuadlet(t *testing.T, name string, managedService bool, ports string) {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	marker := ""
	if managedService {
		marker = CustomServiceQuadletMarker + "\n"
	}
	content := marker + "[Container]\nImage=docker.io/library/redis:7.4.9-alpine\nNetwork=lerd\n" + ports + "\n\n[Service]\nRestart=always\n\n[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(filepath.Join(dir, name+".container"), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readLANQuadlet(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(config.QuadletDir(), name+".container"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}
