//go:build !windows

package cli

import (
	"os/exec"
	"time"
)

// killTray kills any running lerd tray process and waits for it to exit, so a
// replacement launched next does not find the old applet still holding the
// instance lock and quit, leaving no tray at all.
func killTray() {
	signal := func(sig string) {
		for _, pattern := range trayProcessPatterns {
			exec.Command("pkill", "-"+sig, "-f", pattern).Run() //nolint:errcheck
		}
	}
	stopTray(signal, trayRunning, 3*time.Second)
}

func trayRunning() bool {
	for _, pattern := range trayProcessPatterns {
		if exec.Command("pgrep", "-f", pattern).Run() == nil {
			return true
		}
	}
	return false
}
