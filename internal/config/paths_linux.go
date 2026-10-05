package config

import (
	"os"
	"path/filepath"
)

// LaunchAgentsDir is empty on Linux, whose unit dirs follow the XDG vars.
func LaunchAgentsDir() string { return "" }

// ProvidedEnvDir is where env_provider output is kept: under XDG_RUNTIME_DIR,
// which is tmpfs, so secrets never reach disk. Empty when the session has no
// runtime dir; the feature is off there.
func ProvidedEnvDir() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "lerd", "env")
}
