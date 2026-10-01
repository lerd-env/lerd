//go:build windows

package cli

import (
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

const lerdCleanupScript = `# lerd-cleanup.ps1: standalone Lerd uninstaller for Windows.
# Run this from an elevated PowerShell if the lerd binary is already gone and
# you still need to remove containers, the login entry and the DNS rule.

Write-Host "==> Lerd cleanup"

Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -match 'lerd.*(watch|serve-ui|ui)' -and $_.Name -eq 'lerd.exe' } |
  ForEach-Object { Write-Host "  --> Stopping lerd process $($_.ProcessId)"; Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }

if (Get-Command podman -ErrorAction SilentlyContinue) {
  podman ps -a --format '{{.Names}}' 2>$null | Where-Object { $_ -like 'lerd-*' } | ForEach-Object {
    Write-Host "  --> Removing container $_"
    podman rm -f $_ 2>$null | Out-Null
  }
  Write-Host "  --> Removing lerd podman network"
  podman network rm lerd 2>$null | Out-Null
}

Write-Host "  --> Removing login entry"
Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'lerd-autostart' -ErrorAction SilentlyContinue

Write-Host "  --> Removing DNS rule"
Get-DnsClientNrptRule -ErrorAction SilentlyContinue | Where-Object { $_.Comment -eq 'lerd' } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue

$ans = Read-Host "Remove config and data? [y/N]"
if ($ans -match '^(y|yes)$') {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue "$env:USERPROFILE\.config\lerd", "$env:USERPROFILE\.local\share\lerd"
  Write-Host "  --> Config and data removed."
} else {
  Write-Host "  --> Config and data kept."
}

Write-Host "Lerd cleanup complete."
`

// installCleanupScript writes a standalone PowerShell uninstaller next to the
// lerd binary, so the environment can be cleaned up after the binary is gone.
func installCleanupScript() {
	if err := os.MkdirAll(config.BinDir(), 0755); err != nil {
		feedback.Warn("could not create %s: %v", config.BinDir(), err)
		return
	}
	dest := config.BinDir() + string(os.PathSeparator) + "lerd-cleanup.ps1"
	if err := os.WriteFile(dest, []byte(lerdCleanupScript), 0644); err != nil {
		feedback.Warn("could not write lerd-cleanup.ps1: %v", err)
	}
}
