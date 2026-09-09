package cli

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The images are what serves every site under the container runtime, so
// removing them is only ever safe once PHP is running on the host.
func TestRemoveFPMImagesRefusesUnderTheContainerRuntime(t *testing.T) {
	var called []string
	orig := removeFPMImageFn
	removeFPMImageFn = func(v string) error { called = append(called, v); return nil }
	t.Cleanup(func() { removeFPMImageFn = orig })

	if _, err := removeFPMImages(config.PHPRuntimeContainer, []string{"8.3", "8.4"}); err == nil {
		t.Error("removing the images that are serving must be refused")
	}
	if len(called) != 0 {
		t.Errorf("removed %v while the container runtime was serving", called)
	}
}

func TestRemoveFPMImagesRemovesEachInstalledVersion(t *testing.T) {
	var called []string
	orig := removeFPMImageFn
	removeFPMImageFn = func(v string) error { called = append(called, v); return nil }
	t.Cleanup(func() { removeFPMImageFn = orig })

	n, err := removeFPMImages(config.PHPRuntimeNative, []string{"8.3", "8.4"})
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
	orig := removeFPMImageFn
	removeFPMImageFn = func(v string) error {
		if v == "8.3" {
			return errors.New("image is in use")
		}
		return nil
	}
	t.Cleanup(func() { removeFPMImageFn = orig })

	n, err := removeFPMImages(config.PHPRuntimeNative, []string{"8.3", "8.4", "8.5"})
	if err == nil {
		t.Error("the one that failed should be reported")
	}
	if n != 2 {
		t.Errorf("removed %d, want the two that could go", n)
	}
}
