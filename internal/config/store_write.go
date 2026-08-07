package config

import (
	"os"
	"path/filepath"
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
//
// Publishing through a rename is what makes the write atomic, but it also
// replaces the file rather than rewriting it, which a plain os.WriteFile never
// did. Resolving symlinks first keeps a store entry someone pointed at their
// own checkout pointing there, and carrying the existing mode over keeps a
// tightened file tightened.
func publishStoreFile(path string, data []byte, perm os.FileMode) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	if info, err := os.Stat(target); err == nil {
		perm = info.Mode().Perm()
	}
	wrote, err := atomicfile.WriteIfChanged(target, data, perm)
	if err != nil || wrote {
		return err
	}
	now := time.Now()
	return os.Chtimes(target, now, now)
}
