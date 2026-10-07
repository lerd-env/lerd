package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stageFPMQuadlet writes the unit file that makes a PHP version count as
// installed, so a test can tell a version this machine has from one it doesn't.
func stageFPMQuadlet(t *testing.T, version string) {
	t.Helper()
	dir := config.QuadletDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	short := ""
	for _, c := range version {
		if c != '.' {
			short += string(c)
		}
	}
	path := filepath.Join(dir, "lerd-php"+short+"-fpm.container")
	if err := os.WriteFile(path, []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// php:rebuild is the command every surface names when a version is missing: the
// doctor's fix, isolate's image-gap note, the shim's not-installed error. It
// only ever built the image, so on a version this machine had never installed
// the build landed on nothing — php:list kept omitting it, `lerd php` kept
// calling it uninstalled, and the restart at the end failed with "unit not
// found". The rebuild registers the version first now.
func TestRegisterPHPVersionForRebuild_writesAMissingQuadlet(t *testing.T) {
	isolateUnitDir(t)

	var wrote []string
	orig := writeFPMQuadlet
	writeFPMQuadlet = func(version string) (bool, error) {
		wrote = append(wrote, version)
		return true, nil
	}
	t.Cleanup(func() { writeFPMQuadlet = orig })

	if err := registerPHPVersionForRebuild("8.4"); err != nil {
		t.Fatalf("registerPHPVersionForRebuild: %v", err)
	}
	if len(wrote) != 1 || wrote[0] != "8.4" {
		t.Errorf("wrote quadlets for %v, want exactly [8.4]", wrote)
	}
}

// A version that is already installed keeps the unit it has: rewriting it would
// churn systemd on every rebuild for no change.
func TestRegisterPHPVersionForRebuild_leavesAnInstalledVersionAlone(t *testing.T) {
	isolateUnitDir(t)
	stageFPMQuadlet(t, "8.4")

	var wrote []string
	orig := writeFPMQuadlet
	writeFPMQuadlet = func(version string) (bool, error) {
		wrote = append(wrote, version)
		return true, nil
	}
	t.Cleanup(func() { writeFPMQuadlet = orig })

	if err := registerPHPVersionForRebuild("8.4"); err != nil {
		t.Fatalf("registerPHPVersionForRebuild: %v", err)
	}
	if len(wrote) != 0 {
		t.Errorf("rewrote the quadlet for an installed version: %v", wrote)
	}
}
