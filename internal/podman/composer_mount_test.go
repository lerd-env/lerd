package podman

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/composer"
)

// A shell opened inside the container must reach the same composer every lerd
// command runs, not the older one the image was built with.
func TestFPMQuadlet_mountsLerdsComposerOverTheImages(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	phar := composer.PharPath()
	if err := os.MkdirAll(filepath.Dir(phar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(phar, []byte("#!/usr/bin/env php\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	want := "Volume=" + phar + ":/usr/local/bin/composer:ro"
	if !strings.Contains(content, want) {
		t.Errorf("quadlet does not mount lerd's composer (%s):\n%s", want, content)
	}
}

// With no phar on disk the mount would make podman create a directory at
// /usr/local/bin/composer, leaving the container with no composer at all.
func TestFPMQuadlet_leavesTheImagesComposerWhenThePharIsMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	content, err := renderFPMQuadletContent("8.4")
	if err != nil {
		t.Fatalf("renderFPMQuadletContent: %v", err)
	}
	if strings.Contains(content, "/usr/local/bin/composer") {
		t.Errorf("quadlet mounts a composer that is not there:\n%s", content)
	}
}

// The per-version writer updates the quadlet without restarting anything, so a
// container from before the mount has to be spotted by what it is running with.
func TestUnitMissingComposerMount(t *testing.T) {
	fakeInspect(t, "true#/etc/hosts|/home/george|")
	if !UnitMissingComposerMount("lerd-php84-fpm") {
		t.Error("a container without the composer mount is drifted")
	}

	fakeInspect(t, "true#/etc/hosts|/home/george|/usr/local/bin/composer|")
	if UnitMissingComposerMount("lerd-php84-fpm") {
		t.Error("a container already carrying the mount is not drifted")
	}

	fakeInspect(t, "false#")
	if UnitMissingComposerMount("lerd-php84-fpm") {
		t.Error("a stopped container picks the quadlet up when it starts")
	}
}

// Nothing to mount means nothing to restart for.
func TestComposerMountDriftedNeedsAPhar(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	fakeInspect(t, "true#/home/george|")

	if composerMountDrifted("lerd-php84-fpm") {
		t.Error("with no phar downloaded there is no mount to be missing")
	}

	phar := composer.PharPath()
	if err := os.MkdirAll(filepath.Dir(phar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(phar, []byte("#!/usr/bin/env php\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !composerMountDrifted("lerd-php84-fpm") {
		t.Error("with a phar on disk a container without the mount is drifted")
	}
}
