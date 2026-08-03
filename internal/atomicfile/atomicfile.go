// Package atomicfile writes small state files the way lerd's daemons need:
// atomically, and only when the content actually changed.
package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
)

// WriteIfChanged writes data to path via a temp file and rename, so a reader
// never sees a half-written file, and skips the write entirely when path
// already holds exactly these bytes.
//
// The skip is the point. A daemon that re-persists a snapshot on a fixed tick
// spends a write, a rename and a dirtied page every tick even when nothing
// happened, which on a laptop keeps the disk and the writeback path from ever
// settling. Comparing against what is already there costs one read of a small,
// page-cached file. Reports whether it wrote.
func WriteIfChanged(path string, data []byte, perm os.FileMode) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}
