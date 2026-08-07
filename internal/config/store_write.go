package config

import (
	"os"
	"time"

	"github.com/geodro/lerd/internal/atomicfile"
)

// publishStoreFile writes a store-cache file atomically and, when the bytes on
// disk already matched, leaves the file alone but moves its mtime forward.
//
// Both staleness windows in the store are measured from the cached file's
// mtime, and the fetch that refreshes it almost always brings back the bytes
// already there. Skipping that write without restamping would leave the file
// looking permanently stale, turning a once-a-day refetch into one on every
// call.
func publishStoreFile(path string, data []byte, perm os.FileMode) error {
	wrote, err := atomicfile.WriteIfChanged(path, data, perm)
	if err != nil || wrote {
		return err
	}
	now := time.Now()
	return os.Chtimes(path, now, now)
}
