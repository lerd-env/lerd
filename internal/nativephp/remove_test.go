package nativephp

import (
	"os"
	"path/filepath"
	"testing"
)

// seedNativeBuild lays down everything an installed native version owns, so a
// removal test can assert that each piece is gone rather than only the binary.
func seedNativeBuild(t *testing.T, version string) {
	t.Helper()
	files := []string{
		BinaryPath(version),
		BinaryPath(version) + ".version",
		FPMBinaryPath(version),
		ConfPath(version),
		filepath.Join(OverrideDir(version), "99-native.ini"),
		filepath.Join(ModulesDir(version), "xdebug.so"),
		filepath.Join(ShimDir(version), "php"),
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

// stopped records the version whose pool was asked to come down, standing in
// for the launchctl call so the test never boots out a real job.
func stopped(seen *string) func(string) error {
	return func(v string) error {
		*seen = v
		return nil
	}
}

func TestRemoveDeletesEveryPieceOfTheBuild(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", tmp)
	seedNativeBuild(t, "8.1")
	seedNativeBuild(t, "8.2")
	var down string

	if err := removeWith("8.1", stopped(&down)); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if down != "8.1" {
		t.Errorf("the pool for %q was taken down, want 8.1", down)
	}

	for _, p := range []string{
		BinaryPath("8.1"),
		BinaryPath("8.1") + ".version",
		FPMBinaryPath("8.1"),
		ConfPath("8.1"),
		OverrideDir("8.1"),
		ModulesDir("8.1"),
		ShimDir("8.1"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived the removal", p)
		}
	}
	if got := ListInstalled(); len(got) != 1 || got[0] != "8.2" {
		t.Errorf("ListInstalled after removing 8.1 = %v, want [8.2]", got)
	}
}

// Removing what is not there is what a retry looks like, and it has to be
// quiet: the first attempt may have taken the binaries and failed on the pool.
func TestRemoveIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", tmp)
	var down string
	if err := removeWith("8.1", stopped(&down)); err != nil {
		t.Fatalf("Remove on a version with no build: %v", err)
	}
}

// The modules live under a per-version directory shared with nothing else, but
// the parent holds every version, so removing one must not take the tree.
func TestRemoveKeepsOtherVersions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("HOME", tmp)
	seedNativeBuild(t, "8.1")
	seedNativeBuild(t, "8.4")

	var down string
	if err := removeWith("8.1", stopped(&down)); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, p := range []string{
		FPMBinaryPath("8.4"),
		filepath.Join(ModulesDir("8.4"), "xdebug.so"),
		filepath.Join(OverrideDir("8.4"), "99-native.ini"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("removing 8.1 took %s: %v", p, err)
		}
	}
}
