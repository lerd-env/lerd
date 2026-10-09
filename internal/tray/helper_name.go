package tray

import (
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// helperName is the lerd-tray file name beside the lerd binary.
func helperName() string { return config.ExeName("lerd-tray") }

// isHelperBinary reports whether exe is the standalone lerd-tray helper, which
// takes no "tray" subcommand.
func isHelperBinary(exe string) bool {
	return strings.TrimSuffix(filepath.Base(exe), ".exe") == "lerd-tray"
}

func lerdExeName() string { return config.ExeName("lerd") }
