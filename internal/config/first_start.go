package config

import (
	"os"
	"path/filepath"
)

// firstStartPendingPath marks a service whose data directory lerd created and
// that has not become ready since. Only such a directory can be reinitialised:
// it has never held anything a user put there.
func firstStartPendingPath(name string) string {
	return filepath.Join(DataDir(), "first-start-pending", name)
}

// EnsureServiceDataDir creates the data directory of a service, marking its
// first start pending when the directory is new.
func EnsureServiceDataDir(name string) error {
	dir := DataSubDir(name)
	_, statErr := os.Stat(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if os.IsNotExist(statErr) {
		MarkFirstStartPending(name)
	}
	return nil
}

// MarkFirstStartPending records that name has not been ready since lerd
// created its data directory.
func MarkFirstStartPending(name string) {
	p := firstStartPendingPath(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
		_ = os.WriteFile(p, nil, 0o644)
	}
}

// FirstStartPending reports whether name's first start has not finished.
func FirstStartPending(name string) bool {
	_, err := os.Stat(firstStartPendingPath(name))
	return err == nil
}

// ClearFirstStartPending records that name has become ready.
func ClearFirstStartPending(name string) { _ = os.Remove(firstStartPendingPath(name)) }
