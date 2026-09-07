package cli

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func nativeConfig(t *testing.T, mode string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.PHP.Runtime = mode
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
}

// The images are what serves every site under the container runtime, so
// removing them is only ever safe once PHP is running on the host.
func TestRemoveFPMImagesRefusesUnderTheContainerRuntime(t *testing.T) {
	nativeConfig(t, config.PHPRuntimeContainer)
	var called []string
	orig := removeFPMImageFn
	removeFPMImageFn = func(v string) error { called = append(called, v); return nil }
	t.Cleanup(func() { removeFPMImageFn = orig })

	if _, err := RemoveFPMImages([]string{"8.3", "8.4"}); err == nil {
		t.Error("removing the images that are serving must be refused")
	}
	if len(called) != 0 {
		t.Errorf("removed %v while the container runtime was serving", called)
	}
}

func TestRemoveFPMImagesRemovesEachInstalledVersion(t *testing.T) {
	nativeConfig(t, config.PHPRuntimeNative)
	var called []string
	orig := removeFPMImageFn
	removeFPMImageFn = func(v string) error { called = append(called, v); return nil }
	t.Cleanup(func() { removeFPMImageFn = orig })

	n, err := RemoveFPMImages([]string{"8.3", "8.4"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || len(called) != 2 {
		t.Errorf("removed %d images %v, want both", n, called)
	}
}

// One image that will not go is not a reason to keep the rest: the point is to
// reclaim the disk, and a partial reclaim still does that.
func TestRemoveFPMImagesKeepsGoingAfterAFailure(t *testing.T) {
	nativeConfig(t, config.PHPRuntimeNative)
	orig := removeFPMImageFn
	removeFPMImageFn = func(v string) error {
		if v == "8.3" {
			return errors.New("image is in use")
		}
		return nil
	}
	t.Cleanup(func() { removeFPMImageFn = orig })

	n, err := RemoveFPMImages([]string{"8.3", "8.4", "8.5"})
	if err == nil {
		t.Error("the one that failed should be reported")
	}
	if n != 2 {
		t.Errorf("removed %d, want the two that could go", n)
	}
}
