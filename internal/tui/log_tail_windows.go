//go:build windows

package tui

import (
	"context"
	"os/exec"

	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/unitlog"
)

// workerLogCmd follows a unit's output: `podman logs -f` for container units,
// the supervised log file (Get-Content -Wait) for host units.
func workerLogCmd(ctx context.Context, unit string) *exec.Cmd {
	if unitlog.IsContainerUnit(unit) {
		return podman.CmdContext(ctx, "logs", "-f", "--tail", "200", unit)
	}
	return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command",
		"Get-Content -Wait -Tail 200 -LiteralPath '"+unitlog.LogPath(unit)+"'")
}
