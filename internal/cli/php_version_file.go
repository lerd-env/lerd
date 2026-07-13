package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// syncPHPVersionFile pins the version lerd resolved into the project's
// .php-version, so the file, the site registry and the FPM container never
// disagree. link clamps to the framework's supported range, and a project left
// declaring a version lerd refuses to run is a pin other tools still trust.
// A file that already matches is left alone: rewriting it would wake the watcher
// and trigger a pointless queue:restart on every link.
func syncPHPVersionFile(dir, version string) error {
	if version == "" {
		return nil
	}
	path := filepath.Join(dir, ".php-version")
	if current, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(current)) == version {
		return nil
	}
	return os.WriteFile(path, []byte(version+"\n"), 0644)
}
