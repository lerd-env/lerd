package podman

import (
	"os"
	"path/filepath"
	"testing"
)

// stubCustomPHPProbe counts probes so a test can tell a cached answer from a
// fresh one.
func stubCustomPHPProbe(t *testing.T, has bool) *int {
	t.Helper()
	calls := 0
	orig := probeCustomPHPFn
	t.Cleanup(func() { probeCustomPHPFn = orig })
	probeCustomPHPFn = func(string) bool {
		calls++
		return has
	}
	return &calls
}

// stubCustomImageID makes the image look built, with the ID the test names. An
// empty id stands for an image podman does not have.
func stubCustomImageID(t *testing.T, id string) {
	t.Helper()
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	exit := 0
	if id == "" {
		exit = 1
	}
	execCommand = fakeExec(id, "", exit)
}

func TestCustomImageHasPHP_CachesPerImageID(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubCustomImageID(t, "sha256:aaa")
	calls := stubCustomPHPProbe(t, true)

	if !CustomImageHasPHP("app") {
		t.Fatal("first answer = false, want true")
	}
	if !CustomImageHasPHP("app") {
		t.Fatal("second answer = false, want the cached true")
	}
	if *calls != 1 {
		t.Errorf("probes = %d, want the image probed once", *calls)
	}
}

func TestCustomImageHasPHP_ReprobesAfterRebuild(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubCustomImageID(t, "sha256:aaa")
	calls := stubCustomPHPProbe(t, false)

	if CustomImageHasPHP("app") {
		t.Fatal("answer = true, want false for an image without php")
	}
	// A rebuild changes the ID, so the stale answer must not be reused.
	stubCustomImageID(t, "sha256:bbb")
	stubCustomPHPProbe(t, true)
	if !CustomImageHasPHP("app") {
		t.Error("answer = false after a rebuild that added php")
	}
	if *calls != 1 {
		t.Errorf("first probe ran %d times, want once", *calls)
	}
}

// An image that is not built yet is not routed to, and nothing is cached for it.
func TestCustomImageHasPHP_UnbuiltImage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	stubCustomImageID(t, "")
	calls := stubCustomPHPProbe(t, true)

	if CustomImageHasPHP("app") {
		t.Error("answer = true for an image that does not exist")
	}
	if *calls != 0 {
		t.Errorf("probes = %d, want none without an image", *calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "lerd", "container-tools", "app.json")); !os.IsNotExist(err) {
		t.Error("an unbuilt image left a cache file")
	}
}
