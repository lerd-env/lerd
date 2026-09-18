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
