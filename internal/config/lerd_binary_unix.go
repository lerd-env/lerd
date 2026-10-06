//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

// installedLerdBinary is the path to fall back on when the running executable
// is not one anything may record: lerd's own install location, which the shims'
// `[ -x "$LERD" ] || LERD=lerd` line covers if it turns out to be elsewhere.
func installedLerdBinary() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "lerd")
}
