package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Linux bind mount is the host's own directory, so the guard answers without
// starting a container.
func TestContainerSeesHostDirSkipsLinux(t *testing.T) {
	probed := false
	if !containerSeesHostDirOn("linux", t.TempDir(), func(string) bool { probed = true; return false }) {
		t.Error("a Linux path must be reported as visible")
	}
	if probed {
		t.Error("Linux must not be probed")
	}
}

// The Podman Machine VM always shares the host home, so a project under it
// needs no round trip either.
func TestContainerSeesHostDirSkipsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	probed := false
	dir := filepath.Join(home, "Sites")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if !containerSeesHostDirOn("darwin", dir, func(string) bool { probed = true; return false }) {
		t.Error("a path under the host home must be reported as visible")
	}
	if probed {
		t.Error("a path under the host home must not be probed")
	}
}

// The reported failure: the container answers no, because the mount resolves to
// an empty stand-in inside the VM rather than the external drive.
func TestContainerSeesHostDirDetectsUnsharedPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	var seen string
	got := containerSeesHostDirOn("darwin", dir, func(marker string) bool {
		seen = marker
		return false
	})
	if got {
		t.Error("a path the container cannot see must be reported as unshared")
	}
	if filepath.Dir(seen) != dir {
		t.Errorf("probed %q, want a marker inside %q", seen, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the probe left %v behind", entries)
	}
}

// A container that finds the marker is looking at the host's own directory.
func TestContainerSeesHostDirAcceptsSharedPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	if !containerSeesHostDirOn("darwin", dir, func(marker string) bool {
		_, err := os.Stat(marker)
		return err == nil
	}) {
		t.Error("a marker the container can read must report the path as shared")
	}
}

// A directory that cannot take the marker is the create command's problem to
// report, not a mount failure to invent.
func TestContainerSeesHostDirIgnoresUnwritableDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if !containerSeesHostDirOn("darwin", filepath.Join(t.TempDir(), "absent"), func(string) bool { return false }) {
		t.Error("an unwritable directory must not be reported as unshared")
	}
}

// The scaffold must stop before composer runs when the parent is mounted but
// the mount does not reach the host.
func TestPrepareScaffoldParentRejectsUnsharedParent(t *testing.T) {
	stubMountSeams(t, true, false)
	orig := containerSeesHostDir
	containerSeesHostDir = func(string, string) bool { return false }
	t.Cleanup(func() { containerSeesHostDir = orig })

	err := prepareScaffoldParent(filepath.Join(t.TempDir(), "drive", "app"))
	if err == nil {
		t.Fatal("expected an error for a parent the container cannot reach")
	}
	if !strings.Contains(err.Error(), "/Volumes") {
		t.Errorf("error = %v, want it to name the external drive case", err)
	}
}

// composer exits 0 having written the whole project inside the VM, so success
// is only reported once the project is on the host.
func TestScaffoldLanded(t *testing.T) {
	dir := t.TempDir()
	if err := scaffoldLanded(dir); err != nil {
		t.Errorf("scaffoldLanded on an existing directory: %v", err)
	}

	missing := filepath.Join(dir, "new-project")
	err := scaffoldLanded(missing)
	if err == nil {
		t.Fatal("expected an error when the project is not on disk")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error = %v, want it to name the target", err)
	}
}
