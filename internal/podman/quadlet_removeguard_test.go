package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A test that forgot to isolate XDG deleted the developer's real lerd-dns quadlet,
// leaving the container running under a unit systemd no longer had a definition
// for, so Restart=always silently stopped applying. Writes have always been
// guarded; removals were the hole.
func TestRemoveQuadlet_guardsTheRealQuadletDir(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("removing a quadlet from the real dir must panic, not delete the developer's unit")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "real lerd state") {
			t.Errorf("panic should name the problem, got %v", r)
		}
	}()
	_ = RemoveQuadlet("lerd-dns")
}

// The guard must stay out of the way of an isolated test.
func TestRemoveQuadlet_allowsAnIsolatedDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "lerd-probe.container")
	if err := os.WriteFile(path, []byte("[Container]\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := RemoveQuadlet("lerd-probe"); err != nil {
		t.Fatalf("RemoveQuadlet: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an isolated quadlet must still be removed")
	}
}
