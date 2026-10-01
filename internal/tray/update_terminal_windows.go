//go:build !nogui && windows

package tray

import "os/exec"

// openUpdateTerminal opens a console that offers the update, since the tray
// itself has no console to prompt in.
func openUpdateTerminal(latestVer string) {
	script := "Write-Host 'Lerd update available: v" + latestVer + "'; " +
		"if ((Read-Host 'Update now? [y/N]') -match '^[Yy]$') { lerd update }; " +
		"Read-Host 'Press Enter to close'"
	_ = exec.Command("cmd", "/c", "start", "", "powershell", "-NoProfile", "-Command", script).Start()
}
