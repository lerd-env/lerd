//go:build !darwin

package cli

// reclaimedHostHint is empty off macOS: podman runs natively, so a removed
// image frees host disk the moment it goes.
func reclaimedHostHint() string { return "" }
