package config

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockSites guards one read-modify-write of sites.yaml. The flock spans the CLI,
// lerd-ui and the watcher, which each save the whole file, so a write that
// cannot take it fails rather than run unguarded and drop another's edit.
func lockSites() (unlock func(), err error) {
	siteWriteMu.Lock()
	path := filepath.Join(DataDir(), "sites.lock")
	guardRealWrite(path)
	if err := os.MkdirAll(DataDir(), 0755); err != nil {
		siteWriteMu.Unlock()
		return nil, fmt.Errorf("locking the site registry: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		siteWriteMu.Unlock()
		return nil, fmt.Errorf("locking the site registry: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		siteWriteMu.Unlock()
		return nil, fmt.Errorf("locking the site registry: %w", err)
	}
	return func() {
		_ = f.Close() // closing the descriptor releases the flock
		siteWriteMu.Unlock()
	}, nil
}
