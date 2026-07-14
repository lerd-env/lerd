package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/podman"
)

// RewriteNginxQuadlet must preserve the Volume= lines for paths outside $HOME.
// It renders the bundled template, which carries no site mounts, so without
// re-injecting them a site parked outside home loses its bind mount from nginx
// and its docroot becomes unreadable inside the container.
func TestRewriteNginxQuadlet_keepsExtraVolumes(t *testing.T) {
	sandbox := t.TempDir()
	home := filepath.Join(sandbox, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(sandbox, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sandbox, "data"))

	cfgDir := filepath.Join(sandbox, "config", "lerd")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A project parked outside $HOME: the case ExtraVolumePaths exists for.
	const outside = "/srv/apps"
	cfg := "parked_directories:\n    - " + outside + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sandbox, "config", "containers", "systemd"), 0o755); err != nil {
		t.Fatal(err)
	}

	paths := podman.ExtraVolumePaths()
	if len(paths) != 1 || paths[0] != outside {
		t.Fatalf("ExtraVolumePaths() = %v, want [%s]", paths, outside)
	}

	// Seed the quadlet the way RewriteFPMQuadlets does: template + extra mounts.
	tmpl, err := podman.GetQuadletTemplate("lerd-nginx.container")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := podman.WriteQuadletDiff("lerd-nginx", podman.InjectExtraVolumes(tmpl, paths)); err != nil {
		t.Fatal(err)
	}

	quadlet := filepath.Join(sandbox, "config", "containers", "systemd", "lerd-nginx.container")
	want := "Volume=" + outside + ":" + outside + ":rw"
	before, err := os.ReadFile(quadlet)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), want) {
		t.Fatalf("seeded quadlet is missing %q:\n%s", want, before)
	}

	// Saving a global nginx http config rewrites the quadlet (internal/ui/nginx_global.go).
	if _, err := RewriteNginxQuadlet(); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(quadlet)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), want) {
		t.Errorf("RewriteNginxQuadlet dropped the out-of-home mount %q; nginx can no longer read the site:\n%s", want, after)
	}
}
