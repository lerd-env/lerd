package serviceops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// A removed built-in default preset (redis) must reinstall by recreating its
// default quadlet, not fail with the old "collides with the built-in" error and
// not shadow the built-in with a custom-service YAML.
func TestInstallBuiltinPreset_recreatesDefaultQuadlet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	orig := podman.DaemonReloadFn
	t.Cleanup(func() { podman.DaemonReloadFn = orig })
	podman.DaemonReloadFn = func() error { return nil }

	svc, err := resolvePresetForInstall("redis", "")
	if err != nil {
		t.Fatalf("resolving a removed built-in must not error, got: %v", err)
	}
	if svc == nil || svc.Name != "redis" {
		t.Fatalf("expected the redis service, got: %+v", svc)
	}

	if err := registerPreset(svc); err != nil {
		t.Fatalf("registerPreset: %v", err)
	}

	quadlet := filepath.Join(config.QuadletDir(), "lerd-redis.container")
	if _, err := os.Stat(quadlet); err != nil {
		t.Errorf("expected the default quadlet to be recreated: %v", err)
	}
	if config.CustomServiceExists("redis") {
		t.Errorf("a built-in must not be persisted as a custom-service YAML")
	}
}
