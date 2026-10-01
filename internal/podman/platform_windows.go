//go:build windows

package podman

// Containers on Windows are linux/amd64 guests inside the Podman machine, so
// no image swap or platform pin is needed, same as native Linux.

func PlatformPodmanArgs(_, _ string) string { return "" }

func PlatformPullArgs(_ string) []string { return nil }

func PlatformImage(image string) string { return image }
