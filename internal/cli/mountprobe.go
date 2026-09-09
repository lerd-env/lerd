package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/podman"
)

// containerSeesHostDir is the seam the scaffold guard calls; tests replace it.
var containerSeesHostDir = probeContainerSeesHostDir

// probeContainerSeesHostDir reports whether dir, bind-mounted into the PHP
// container, is really the host's own directory. On macOS every mount source is
// resolved inside the Podman Machine VM, which shares only the host trees it was
// given at init. A path it does not share becomes an empty stand-in in the VM,
// so a container writing there succeeds and the files never appear on the Mac
// (issue #1725). Asking the container is deliberate: the reasons a share can be
// missing are several, and the symptom is the same for all of them.
func probeContainerSeesHostDir(dir, phpVersion string) bool {
	return containerSeesHostDirOn(runtime.GOOS, dir, fpmSeesFile(dir, phpVersion))
}

// containerSeesHostDirOn is the platform-free core: it drops a marker on the
// host and asks sees whether the container finds it. A Linux bind mount is the
// host's directory by construction, and the VM always shares the host home, so
// neither case is worth a container round trip. A marker that cannot be written
// leaves the answer at yes, since an unwritable directory is a problem the
// create command itself will report in its own terms.
func containerSeesHostDirOn(goos, dir string, sees func(marker string) bool) bool {
	if goos != "darwin" {
		return true
	}
	if home, _ := os.UserHomeDir(); home != "" && (dir == home || strings.HasPrefix(dir, strings.TrimSuffix(home, "/")+"/")) {
		return true
	}
	marker := filepath.Join(dir, fmt.Sprintf(".lerd-mount-probe-%d", os.Getpid()))
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		return true
	}
	defer os.Remove(marker)
	return sees(marker)
}

// fpmSeesFile asks the PHP-FPM container serving dir whether a host path is
// there. Under the native runtime there is no container and no VM between the
// two, so the question is already answered. Anything that stops the check from
// running answers yes for the same reason as above: this guard exists to catch
// a mount that lies, not to turn every podman hiccup into a scaffold failure.
func fpmSeesFile(dir, phpVersion string) func(string) bool {
	return func(marker string) bool {
		if _, native := nativeRuntimeVersion(dir); native {
			return true
		}
		_, container, err := ensureFPMRunning(dir, phpVersion, fpmContainerForDir(dir, phpVersion))
		if err != nil {
			return true
		}
		return podman.Cmd("exec", container, "test", "-e", marker).Run() == nil
	}
}
