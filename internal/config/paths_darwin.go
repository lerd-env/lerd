package config

import (
	"os"
	"path/filepath"
)

// LaunchAgentsDir returns the directory macOS keeps lerd's launchd units in.
// It follows HOME, which is why isolating only the XDG vars leaves it exposed.
func LaunchAgentsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents")
}

// ProvidedEnvDir is empty on macOS: there is no tmpfs runtime dir to keep
// env_provider secrets off disk, so the feature is off.
func ProvidedEnvDir() string { return "" }
