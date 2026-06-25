package podman

import (
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// On macOS the host DB is reached over TCP (gvproxy's host.containers.internal),
// so there is no socket directory to bind-mount into FPM — hostDBSocketDirs must
// short-circuit to nil rather than inject a dead mount.
func TestHostDBSocketDirs_skippedOnMacOS(t *testing.T) {
	defer config.SetHostDBGOOSForTest("darwin")()
	if got := hostDBSocketDirs(); got != nil {
		t.Errorf("hostDBSocketDirs() on macOS = %v, want nil (host DB uses TCP, no socket mount)", got)
	}
}

// The directory bind-mounted into FPM differs by engine: for MySQL the socket is
// a FILE, so its PARENT directory is mounted; for Postgres the socket path is
// already the DIRECTORY that holds .s.PGSQL.<port>, so it is mounted as-is.
func TestHostDBSocketDirs_dirVsFilePerEngine(t *testing.T) {
	defer config.SetHostDBGOOSForTest("linux")()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	// Postgres host site: socket path is a directory, mounted itself.
	pgDir := t.TempDir()
	pgSite := t.TempDir()
	if err := config.SaveProjectConfig(pgSite, &config.ProjectConfig{
		DB: config.ProjectDB{External: true, Service: "postgres", Socket: pgDir},
	}); err != nil {
		t.Fatal(err)
	}

	// MySQL host site: socket path is a file, so its parent directory is mounted.
	mysqlParent := t.TempDir()
	mysqlSock := filepath.Join(mysqlParent, "mysqld.sock")
	mysqlSite := t.TempDir()
	if err := config.SaveProjectConfig(mysqlSite, &config.ProjectConfig{
		DB: config.ProjectDB{External: true, Service: "mysql", Socket: mysqlSock},
	}); err != nil {
		t.Fatal(err)
	}

	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		{Name: "pg", Path: pgSite},
		{Name: "my", Path: mysqlSite},
	}}); err != nil {
		t.Fatal(err)
	}

	dirs := hostDBSocketDirs()
	has := func(want string) bool {
		for _, d := range dirs {
			if d == want {
				return true
			}
		}
		return false
	}
	if !has(pgDir) {
		t.Errorf("hostDBSocketDirs() = %v, want the Postgres socket dir %q mounted as-is", dirs, pgDir)
	}
	if !has(mysqlParent) {
		t.Errorf("hostDBSocketDirs() = %v, want the MySQL socket's parent %q mounted", dirs, mysqlParent)
	}
}
