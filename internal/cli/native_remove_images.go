package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// removeFPMImageFn is the removal, swappable so the decision around it can be
// tested without a container store.
var removeFPMImageFn = podman.RemoveFPMImage

// RemoveFPMImages deletes the shared PHP-FPM images, reclaiming the disk they
// hold once PHP is running on the host. Returns how many went.
//
// Refused under the container runtime: there those images are what serves every
// site, and removing them would take the whole install down until each was
// built again.
func RemoveFPMImages(versions []string) (int, error) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return 0, err
	}
	if cfg.PHPRuntimeMode() != config.PHPRuntimeNative {
		return 0, fmt.Errorf("the PHP-FPM images are what serves every site on the container runtime; switch to the native runtime before removing them")
	}

	removed := 0
	var failed []string
	for _, v := range versions {
		if err := removeFPMImageFn(v); err != nil {
			failed = append(failed, v)
			continue
		}
		removed++
	}
	// A partial reclaim is still a reclaim, so the ones that went are kept and
	// only the rest are named.
	if len(failed) > 0 {
		return removed, fmt.Errorf("could not remove the image for php %s", strings.Join(failed, ", "))
	}
	return removed, nil
}
