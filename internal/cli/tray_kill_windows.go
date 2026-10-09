//go:build windows

package cli

import (
	"os"
	"os/exec"
	"strconv"
)

// killTray kills any running lerd tray process: the lerd-tray.exe helper, and
// a `lerd.exe tray` launched directly. The command-line match skips this
// process so `lerd tray off` does not take itself out.
func killTray() {
	_ = exec.Command("taskkill", "/F", "/IM", "lerd-tray.exe").Run()
	script := `Get-CimInstance Win32_Process -Filter "Name='lerd.exe'" | ` +
		`Where-Object { $_.ProcessId -ne ` + strconv.Itoa(os.Getpid()) + ` -and $_.CommandLine -match 'lerd(\.exe)?"?\s+tray(\s+--mono(=\w+)?)?\s*$' } | ` +
		`ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`
	_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Run()
}
