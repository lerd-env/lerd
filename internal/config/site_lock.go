package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// lockSites guards one read-modify-write of sites.yaml. The flock spans the CLI,
// lerd-ui and the watcher, which each save the whole file, so a write that
// cannot take it fails rather than run unguarded and drop another's edit.
func lockSites() (unlock func(), err error) {
	siteWriteMu.Lock()
	release, err := flockFile("sites.lock")
	if err != nil {
		siteWriteMu.Unlock()
		return nil, fmt.Errorf("locking the site registry: %w", err)
	}
	return func() {
		release()
		siteWriteMu.Unlock()
	}, nil
}

var devServerPortMu sync.Mutex

// LockDevServerPorts serialises picking a dev server's port across processes:
// a `lerd restart` and the idle engine's wake can start the same server at once.
func LockDevServerPorts() (unlock func(), err error) {
	devServerPortMu.Lock()
	release, err := flockFile("dev-server-ports.lock")
	if err != nil {
		devServerPortMu.Unlock()
		return nil, fmt.Errorf("locking the dev server ports: %w", err)
	}
	return func() {
		release()
		devServerPortMu.Unlock()
	}, nil
}

// flockFile takes an exclusive lock on name in the data dir.
func flockFile(name string) (release func(), err error) {
	path := filepath.Join(DataDir(), name)
	guardRealWrite(path)
	if err := os.MkdirAll(DataDir(), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil // closing the descriptor releases the flock
}
