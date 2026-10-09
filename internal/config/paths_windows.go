package config

import "os"

// LaunchAgentsDir is empty on Windows, which has no launchd.
func LaunchAgentsDir() string { return "" }

// ProvidedEnvDir is empty on Windows: there is no tmpfs runtime dir to keep
// env_provider secrets off disk, so the feature is off.
func ProvidedEnvDir() string { return "" }

// osBaseDir is the Windows default for one kind of base directory.
func osBaseDir(kind string) (string, bool) { return windowsBase(kind, os.Getenv) }
