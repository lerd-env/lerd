//go:build darwin

package cli

// reclaimedHostHint says where the freed disk actually went. On macOS podman
// runs inside a VM whose disk image is sparse and only ever grows, so blocks
// freed in the guest stay charged to the host until the image is trimmed.
func reclaimedHostHint() string {
	return "That is free inside the Podman Machine VM. Run `lerd machine reclaim` to return it to macOS."
}
