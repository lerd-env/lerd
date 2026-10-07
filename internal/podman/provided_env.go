package podman

import (
	"os"
	"runtime"

	"github.com/geodro/lerd/internal/config"
)

// ProvidedEnvVMDir is where macOS keeps env_provider output: tmpfs inside the
// Podman Machine VM, which is where the FPM containers run, so the values never
// reach the Mac's disk.
const ProvidedEnvVMDir = "/run/lerd/env"

// providedEnvLines mounts the tmpfs env_provider dir into FPM, read-only.
func providedEnvLines() (mount, execStartPre string) {
	native := false
	if cfg, err := config.LoadGlobal(); err == nil {
		native = cfg.PHPRuntimeMode() == config.PHPRuntimeNative
	}
	return providedEnvLinesFor(runtime.GOOS, native, os.Getenv("XDG_RUNTIME_DIR"))
}

// providedEnvLinesFor renders the mount per platform. On Linux the dir is the
// user's runtime dir, created before start because a reboot empties tmpfs and
// podman refuses a missing bind source. On macOS it is the VM's /run, which lerd
// start creates over ssh since the launchd units have no ExecStartPre. Empty
// where the feature is off: no runtime dir, or PHP running natively on the host.
func providedEnvLinesFor(goos string, native bool, xdgRuntimeDir string) (mount, execStartPre string) {
	switch {
	case goos == "linux" && xdgRuntimeDir != "":
		return "Volume=%t/lerd/env:" + config.ProvidedEnvContainerDir + ":ro",
			"ExecStartPre=/bin/mkdir -p -m 0700 %t/lerd/env"
	case goos == "darwin" && !native:
		return "Volume=" + ProvidedEnvVMDir + ":" + config.ProvidedEnvContainerDir + ":ro", ""
	}
	return "", ""
}
