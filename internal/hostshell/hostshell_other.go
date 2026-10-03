//go:build !windows

package hostshell

import (
	"context"
	"os/exec"
)

// Command runs shell through sh -c.
func Command(ctx context.Context, shell string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", shell)
}

// Hide has nothing to do outside Windows, where no console window opens.
func Hide(*exec.Cmd) {}
