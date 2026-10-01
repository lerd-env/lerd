package tray

import (
	"path/filepath"
	"runtime"
	"strings"
)

// helperName is the lerd-tray file name beside the lerd binary.
func helperName() string {
	if runtime.GOOS == "windows" {
		return "lerd-tray.exe"
	}
	return "lerd-tray"
}

// isHelperBinary reports whether exe is the standalone lerd-tray helper, which
// takes no "tray" subcommand.
func isHelperBinary(exe string) bool {
	return strings.TrimSuffix(filepath.Base(exe), ".exe") == "lerd-tray"
}

func lerdExeName() string {
	if runtime.GOOS == "windows" {
		return "lerd.exe"
	}
	return "lerd"
}
