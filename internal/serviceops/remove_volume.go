package serviceops

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// Seams over the podman volume commands, so the removal logic is testable
// without a machine.
var (
	volumeExistsFn = func(vol string) bool { return podman.Cmd("volume", "exists", vol).Run() == nil }
	volumeExportFn = func(vol, path string) error {
		return podman.Cmd("volume", "export", "--output", path, vol).Run()
	}
	volumeRemoveFn = func(vol string) error { return podman.Cmd("volume", "rm", "-f", vol).Run() }
)

// removeServiceData clears a service's data the recoverable way: a host
// directory is renamed aside, and a named volume (Windows) is exported to a
// tarball beside where that directory would be, and only then deleted. If the
// export fails the volume is kept, since deleting it would break the promise
// that removal can be undone.
func removeServiceData(name string, namedVolume bool) error {
	if !namedVolume {
		return renameDataAside(config.DataSubDir(name))
	}
	vol := config.DataVolumeName(name)
	if !volumeExistsFn(vol) {
		return nil
	}
	tar := fmt.Sprintf("%s.pre-remove-%d.tar", config.DataSubDir(name), time.Now().UnixNano())
	if err := os.MkdirAll(filepath.Dir(tar), 0755); err != nil {
		return err
	}
	if err := volumeExportFn(vol, tar); err != nil {
		return fmt.Errorf("export volume %s: %w", vol, err)
	}
	return volumeRemoveFn(vol)
}
