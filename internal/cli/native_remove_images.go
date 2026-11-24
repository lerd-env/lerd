package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
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
	return removeFPMImages(cfg.PHPRuntimeMode(), versions)
}

// removeFPMImages is RemoveFPMImages with the runtime handed to it, so what it
// decides can be checked on any platform. The exported one reads the mode, and
// off macOS that is always container, which would otherwise leave this testable
// on one platform only.
func removeFPMImages(mode string, versions []string) (int, error) {
	if mode != config.PHPRuntimeNative {
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

// fpmImageExistsFn reports whether a version's PHP-FPM image is still on disk,
// swappable so the offer can be tested without podman.
var fpmImageExistsFn = func(v string) bool { return podman.ImageExists(podman.FPMImageName(v)) }

// confirmImageRemovalFn asks the question, swappable so a test can answer it.
var confirmImageRemovalFn = feedback.Confirm

// leftoverFPMImages narrows the installed versions to those whose image is
// still there, so the offer never names one already gone.
func leftoverFPMImages(versions []string) []string {
	var out []string
	for _, v := range versions {
		if fpmImageExistsFn(v) {
			out = append(out, v)
		}
	}
	return out
}

// offerFPMImageRemoval is the terminal's version of the dashboard's checkbox:
// once PHP runs on the host it asks to remove the images, or with no one to
// ask, names the flag that does. remove true is that flag, given up front.
func offerFPMImageRemoval(mode string, versions []string, remove bool) {
	if mode != config.PHPRuntimeNative {
		return
	}
	left := leftoverFPMImages(versions)
	if len(left) == 0 {
		return
	}
	list := strings.Join(left, ", ")
	if !remove {
		if !feedback.StdinInteractive() {
			feedback.Note("The PHP-FPM images for php " + list + " are no longer used. Remove them with: lerd php:runtime native --remove-images")
			return
		}
		remove = confirmImageRemovalFn("Remove the PHP-FPM images for php "+list+"? Nothing uses them on the native runtime; switching back rebuilds them.", false)
		if !remove {
			return
		}
	}
	n, err := removeFPMImages(mode, left)
	if n > 0 {
		feedback.Done(fmt.Sprintf("removed %d PHP-FPM image(s)", n))
		if hint := reclaimedHostHint(); hint != "" {
			feedback.Note(hint)
		}
	}
	if err != nil {
		feedback.Warn("%v", err)
	}
}
