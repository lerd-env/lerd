package cli

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// A failed build used to end in "✓ PHP 8.6 image rebuilt" with exit 0, and it
// stored the Containerfile hash, so lerd then treated the stale image as current.
func TestPhpRebuild_failedBuildExitsNonZeroAndKeepsTheHash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	stageFPMQuadlet(t, "8.4")

	origBuild, origHash, origRestart := rebuildFPMImageTo, storeFPMHash, restartRebuiltPHP
	t.Cleanup(func() { rebuildFPMImageTo, storeFPMHash, restartRebuiltPHP = origBuild, origHash, origRestart })
	rebuildFPMImageTo = func(string, bool, io.Writer) error { return errors.New("manifest unknown") }
	hashStored := false
	storeFPMHash = func() error { hashStored = true; return nil }
	restartRebuiltPHP = func([]string) {}

	cmd := NewPhpRebuildCmd()
	cmd.SetArgs([]string{"8.4", "--local"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("a failed build must exit non-zero")
	}
	if hashStored {
		t.Error("the image hash was stored after a failed build, so the stale image counts as current")
	}
}

// While its image was missing, the unit crash-looped into systemd's start limit,
// so the restart after a successful build was refused with start-limit-hit.
func TestRestartOnNewImage_clearsTheFailedStateFirst(t *testing.T) {
	origReset, origRestart := resetFailedUnit, restartFPMOnNewImage
	t.Cleanup(func() { resetFailedUnit, restartFPMOnNewImage = origReset, origRestart })
	var calls []string
	resetFailedUnit = func(unit string) { calls = append(calls, "reset-failed "+unit) }
	restartFPMOnNewImage = func(unit string) error { calls = append(calls, "restart "+unit); return nil }

	if err := restartOnNewImage("lerd-php86-fpm"); err != nil {
		t.Fatal(err)
	}
	want := []string{"reset-failed lerd-php86-fpm", "restart lerd-php86-fpm"}
	if strings.Join(calls, ", ") != strings.Join(want, ", ") {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}
