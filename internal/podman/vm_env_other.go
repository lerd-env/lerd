//go:build !windows

package podman

// guestExecEnv is empty off Windows: rootless podman maps the user to root in
// the container, so the project already looks like root's own to git.
var guestExecEnv []string
