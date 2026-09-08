package php

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
)

// Seams so the choice can be tested without either kind of install present.
var (
	nativeRuntimeFn = func() bool {
		cfg, err := config.LoadGlobal()
		return err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative
	}
	nativeInstalledFn    = nativephp.ListInstalled
	containerInstalledFn = ListInstalled
)

// InstalledForRuntime lists the PHP versions this install can actually serve:
// the host builds under the native runtime, the FPM units otherwise.
//
// The two lists genuinely differ, and in both directions: a version can have a
// host build and no image, or an image and no host build. Listing versions from
// one and then answering questions about them from the other is what left the
// dashboard offering a version whose every endpoint then answered 404, and
// showing another as permanently stopped.
func InstalledForRuntime() ([]string, error) {
	if nativeRuntimeFn() {
		return nativeInstalledFn(), nil
	}
	return containerInstalledFn()
}
