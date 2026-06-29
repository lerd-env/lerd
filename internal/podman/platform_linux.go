//go:build linux

package podman

// PlatformPodmanArgs is a no-op on Linux. linux/amd64 pulls the upstream
// postgis manifest natively; linux/arm64 hits the same gap as macOS but
// lerd has not historically shipped there. Add a runtime.GOARCH guard then.
func PlatformPodmanArgs(_, _ string) string {
	return ""
}

// PlatformPullArgs is a no-op on Linux (see PlatformPodmanArgs).
func PlatformPullArgs(_ string) []string {
	return nil
}

// PlatformImage is a no-op on Linux. linux/amd64 runs postgis natively;
// linux/arm64 would hit the same gap as Apple Silicon, so add the
// rewriteArm64Image swap behind a runtime.GOARCH guard then.
func PlatformImage(image string) string {
	return image
}
