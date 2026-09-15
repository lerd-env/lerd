package nativephp

import (
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// runtimeDir holds everything a version's pool is generated into: the
// php-fpm.conf it is started with, the ini fragment it scans last, and the
// shim dir workers reach php through.
func runtimeDir(version string) string {
	return filepath.Join(config.RunDir(), "native", version)
}

// Remove uninstalls a version's native build: the pool comes down, then the
// generated runtime and the files on disk go, so ListInstalled stops reporting
// a version nothing can serve with.
func Remove(version string) error { return removeWith(version, Stop) }

// removeWith is Remove with the launchd teardown injected, so a test can prove
// what is deleted without booting a job out of the developer's own session.
// Every step tolerates an absent file, because a retry after a half-finished
// removal is the normal way this is called a second time.
func removeWith(version string, stop func(string) error) error {
	if err := stop(version); err != nil {
		return err
	}
	paths := []string{
		runtimeDir(version),
		filepath.Dir(ModulesDir(version)),
		BinaryPath(version),
		BinaryPath(version) + ".version",
		FPMBinaryPath(version),
		FPMBinaryPath(version) + ".version",
		LogPath(version),
	}
	for _, p := range paths {
		config.GuardRealWrite(p)
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}
