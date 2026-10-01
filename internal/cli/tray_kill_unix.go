//go:build !windows

package cli

import "os/exec"

// killTray kills any running lerd tray process.
func killTray() {
	for _, pattern := range trayProcessPatterns {
		exec.Command("pkill", "-f", pattern).Run() //nolint:errcheck
	}
}
