package config

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockSites guards one read-modify-write of sites.yaml. The flock spans the CLI,
// lerd-ui and the watcher, which each save the whole file; the cache is dropped
// since another process's save can keep the size and share the mtime tick.
func lockSites() (unlock func()) {
	siteWriteMu.Lock()
	invalidateSitesCache()
	path := filepath.Join(DataDir(), "sites.lock")
	guardRealWrite(path)
	_ = os.MkdirAll(DataDir(), 0755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return siteWriteMu.Unlock
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	return func() {
		_ = f.Close() // closing the descriptor releases the flock
		siteWriteMu.Unlock()
	}
}
