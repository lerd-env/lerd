//go:build darwin

package cli

import "github.com/geodro/lerd/internal/podman"

// Wire the MCP/exec hot-path self-heal to the same machine restart the service
// start pass uses. On a post-sleep stall, podman.EnsureMachineResponsive calls
// this to stop+restart the VM and wait for its API socket, then retries once.
func init() { podman.MachineHeal = restartPodmanMachineForHeal }
