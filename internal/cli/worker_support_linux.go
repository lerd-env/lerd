//go:build linux

package cli

import "github.com/geodro/lerd/internal/config"

// workerSupportedOnPlatform reports whether the given worker can run on the
// current host. Linux supports every shape — host workers go through fnm +
// systemd user units; scheduled workers via .timer + Type=oneshot — so the
// gate is unconditionally permissive.
//
// This is a package var (not a function) so tests can substitute it to
// exercise the unsupported path without a darwin build. The macOS build
// installs the real darwin variant in worker_support_darwin.go.
var workerSupportedOnPlatform = func(_ config.FrameworkWorker) (bool, string) {
	return true, ""
}

// enableArmsBootOnly is true on Linux: `systemctl --user enable` only writes the
// default.target.wants symlink, so skipping it while autostart is off costs the
// unit nothing at runtime. A package var, like workerSupportedOnPlatform, so a
// test can exercise both platforms' behaviour from either build.
var enableArmsBootOnly = true
