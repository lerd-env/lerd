package podman

import (
	"errors"
	"io/fs"
	"os"
)

// ReadUnshared reads a file that may sit inside a container-owned directory.
// Service data dirs are owned by the container's mapped uid with mode 0700, so
// the host user cannot even traverse them; the read has to happen inside the
// user namespace. The direct read comes first so host-readable paths cost no
// process spawn, and the original error is returned when the fallback also
// fails, keeping "absent" distinguishable from "unreadable".
func ReadUnshared(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return data, err
	}
	out, unshareErr := Cmd("unshare", "cat", path).Output()
	if unshareErr != nil {
		return nil, err
	}
	return out, nil
}
