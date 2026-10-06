package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

const autostartFixture = "[Container]\nImage=x\n\n[Install]\nWantedBy=default.target\n"

func seedAutostartQuadlet(t *testing.T, unit string) string {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	path := filepath.Join(dir, unit+".container")
	if err := os.WriteFile(path, []byte(autostartFixture), 0o644); err != nil {
		t.Fatalf("seed quadlet: %v", err)
	}
	return path
}

func TestSetQuadletAutostart_stripAndRestore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := seedAutostartQuadlet(t, "lerd-redis")

	if !SetQuadletAutostart("lerd-redis", false) {
		t.Fatal("strip reported no change")
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "[Install]") {
		t.Errorf("[Install] not stripped:\n%s", b)
	}
	if SetQuadletAutostart("lerd-redis", false) {
		t.Error("second strip reported a change")
	}

	if !SetQuadletAutostart("lerd-redis", true) {
		t.Fatal("restore reported no change")
	}
	if b, _ := os.ReadFile(path); string(b) != autostartFixture {
		t.Errorf("restore did not round-trip:\n%s", b)
	}
}

func TestSetQuadletAutostart_restoreHonoursGlobalDisabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.Autostart.Disabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	path := seedAutostartQuadlet(t, "lerd-redis")
	SetQuadletAutostart("lerd-redis", false)

	SetQuadletAutostart("lerd-redis", true)
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "[Install]") {
		t.Errorf("restore re-armed a unit with global autostart off:\n%s", b)
	}
}

func TestSetQuadletAutostart_missingQuadletNoop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if SetQuadletAutostart("lerd-redis", false) {
		t.Error("missing quadlet reported a change")
	}
}

// A slept service's quadlet rewritten by install or an update must stay off
// the boot target, or the next reboot wakes it.
func TestWriteQuadletDiffKeepsSleepingServiceOffBoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.SetServiceIdleSuspended("redis", true); err != nil {
		t.Fatalf("flag: %v", err)
	}

	if _, err := WriteQuadletDiff("lerd-redis", autostartFixture); err != nil {
		t.Fatalf("WriteQuadletDiff: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(config.QuadletDir(), "lerd-redis.container"))
	if strings.Contains(string(b), "[Install]") {
		t.Errorf("sleeping service kept [Install]:\n%s", b)
	}
}
