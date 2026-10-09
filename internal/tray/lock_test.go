//go:build !nogui

package tray

import (
	"runtime"
	"testing"
)

// The lock file is only referenced by acquireLock, so a garbage collection
// used to close it and free the lock for the next start to take.
func TestAcquireLock_SurvivesGarbageCollection(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Cleanup(func() {
		if lockFile != nil {
			lockFile.Close()
			lockFile = nil
		}
	})

	if !acquireLock() {
		t.Fatal("the first instance must get the lock")
	}
	runtime.GC()
	runtime.GC()
	if acquireLock() {
		t.Error("a second instance got the lock after a GC, so two trays run")
	}
}
