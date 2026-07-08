package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// TestServiceList_excludesRemovedDefaultPreset covers the "a removed default
// preset lingers in the list" bug: `service list` gates default presets on
// ServiceInstalled (the #678 truth), so a default whose quadlet is gone drops
// out of the installed list while an installed sibling stays.
func TestServiceList_excludesRemovedDefaultPreset(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	// Install one default (redis) by writing its quadlet; leave mysql absent
	// (the removed default).
	quadletDir := config.QuadletDir()
	if err := os.MkdirAll(quadletDir, 0o755); err != nil {
		t.Fatal(err)
	}
	redisQuadlet := filepath.Join(quadletDir, "lerd-redis.container")
	if err := os.WriteFile(redisQuadlet, []byte("[Container]\nImage=docker.io/library/redis:7\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	restore := feedback.SetTestWriter(&buf)
	defer restore()

	if err := newServiceListCmd().Execute(); err != nil {
		t.Fatalf("service list: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "redis") {
		t.Errorf("installed default 'redis' should be listed; got:\n%s", out)
	}
	if strings.Contains(out, "mysql") {
		t.Errorf("removed default 'mysql' should not appear in the list; got:\n%s", out)
	}
}
