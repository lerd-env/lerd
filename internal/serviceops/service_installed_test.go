package serviceops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// TestServiceInstalled_quadletIsSourceOfTruth covers the orphan-quadlet case
// that motivated the helper: a service whose YAML config has been deleted but
// whose .container quadlet still exists must still report as installed,
// matching the real runtime state podman sees.
func TestServiceInstalled_quadletIsSourceOfTruth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	if ServiceInstalled("mysql") {
		t.Fatalf("expected mysql not installed on a clean tree")
	}

	quadletDir := config.QuadletDir()
	if err := os.MkdirAll(quadletDir, 0o755); err != nil {
		t.Fatalf("mkdir quadlet dir: %v", err)
	}
	quadletPath := filepath.Join(quadletDir, "lerd-mysql.container")
	if err := os.WriteFile(quadletPath, []byte("[Container]\nImage=docker.io/library/mysql:8.4\n"), 0o644); err != nil {
		t.Fatalf("write quadlet: %v", err)
	}

	if !ServiceInstalled("mysql") {
		t.Fatalf("expected mysql installed when quadlet exists, even with no YAML")
	}

	if _, err := config.LoadCustomService("mysql"); err == nil {
		t.Fatalf("test precondition broken: YAML should not exist for this scenario")
	}
}
