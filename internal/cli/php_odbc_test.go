package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// list probes each driver inside the image with the generated registry bind-mounted,
// so a registry that is not on disk makes podman refuse the run and every driver
// reads back as "not visible to the container", blaming the user's driver path for
// a file lerd owns. add seeds it through applyODBCChange; list has to seed it too.
func TestPhpOdbcListSeedsAMissingRegistry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	driver := filepath.Join(tmp, "libodbcHDB.so")
	if err := os.WriteFile(driver, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		c.SetODBCDriver(config.ODBCDriver{Name: "HDBODBC", Driver: driver})
	}); err != nil {
		t.Fatalf("UpdateGlobal: %v", err)
	}
	if err := os.Remove(config.OdbcInstFile()); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	cmd := newPhpOdbcListCmd()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("php:odbc list: %v", err)
	}

	info, err := os.Stat(config.OdbcInstFile())
	if err != nil {
		t.Fatalf("list left the registry missing, so its probe mounts nothing: %v", err)
	}
	if info.IsDir() {
		t.Error("registry is a directory, the bind-mount source must be a regular file")
	}
}

// The ostree images keep home at /var/home behind a /home symlink, and they do
// not agree on which spelling lands in passwd: Silverblue's is /var/home/you,
// Bazzite's is /home/you. The quadlet mounts %h, so a driver under home has to
// be stored the way %h spells it whichever way the symlink runs, or the registry
// names a path that does not exist inside the container.
func TestOdbcDriverPathNamesHomeTheWayTheQuadletMountsIt(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "var", "home", "me")
	drv := filepath.Join(realHome, "drv")
	if err := os.MkdirAll(drv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drv, "libodbcHDB.so"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "var", "home"), filepath.Join(root, "home")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	linkHome := filepath.Join(root, "home", "me")

	for _, tc := range []struct {
		name string
		home string // what passwd says, i.e. what %h expands to
		give string // what the user types
	}{
		{"canonical home, driver given through the link", realHome, filepath.Join(linkHome, "drv", "libodbcHDB.so")},
		{"linked home, driver given canonically", linkHome, filepath.Join(realHome, "drv", "libodbcHDB.so")},
		{"linked home, driver given through the link", linkHome, filepath.Join(linkHome, "drv", "libodbcHDB.so")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			got, err := odbcDriverPath(tc.give)
			if err != nil {
				t.Fatalf("odbcDriverPath: %v", err)
			}
			want := filepath.Join(tc.home, "drv", "libodbcHDB.so")
			if got != want {
				t.Errorf("odbcDriverPath = %q, want %q so it matches the %%h mount", got, want)
			}
		})
	}
}

// A driver outside home is mounted by lerd at its own path, so there the
// resolved spelling is the one that has to be stored.
func TestOdbcDriverPathResolvesOutsideHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	if err := os.MkdirAll(filepath.Join(root, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "var", "opt", "drv")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "libodbcHDB.so"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "var", "opt"), filepath.Join(root, "opt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := odbcDriverPath(filepath.Join(root, "opt", "drv", "libodbcHDB.so"))
	if err != nil {
		t.Fatalf("odbcDriverPath: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(real, "libodbcHDB.so"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("odbcDriverPath = %q, want the resolved path %q", got, want)
	}
}
