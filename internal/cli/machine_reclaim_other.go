//go:build !darwin

package cli

import "fmt"

// runMachineReclaim is macOS-only: Linux podman writes to the host filesystem
// directly, so there is no VM disk image holding freed blocks.
func runMachineReclaim() error {
	fmt.Println("lerd machine reclaim is only supported on macOS (Podman Machine). On Linux, podman runs natively without a VM disk image.")
	return nil
}
