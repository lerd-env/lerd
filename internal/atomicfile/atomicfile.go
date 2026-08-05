// Package atomicfile writes small state files the way lerd's daemons need:
// atomically, and only when the content actually changed.
package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
)

// WriteIfChanged writes data to path via a unique temp file, an fsync and a
// rename, so a reader never sees a half-written file and a crash never loses a
// published one, and skips the write entirely when path already holds exactly
// these bytes.
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
	if err := writeAtomic(path, data, perm); err != nil {
		return false, err
	}
	return true, nil
}

// writeAtomic writes through a uniquely named temp in the target's directory,
// fsyncs it, renames it into place and fsyncs the directory entry. The unique
// name is what makes concurrent writers safe: with a fixed path.tmp two of them
// truncate and fill the same file, and the rename publishes their merge.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	discard := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if _, err := f.Write(data); err != nil {
		return discard(err)
	}
	if err := f.Chmod(perm); err != nil {
		return discard(err)
	}
	if err := f.Sync(); err != nil {
		return discard(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return syncDir(dir)
}

// syncDir flushes the directory entry so a crash right after the rename can't
// lose the published file even though its bytes were already fsynced.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
