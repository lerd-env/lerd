//go:build windows

package podman

// guestExecEnv is set on every exec into a container. The PHP container runs as
// root while a project under /mnt belongs to the machine's user, so git (and
// composer through it) would refuse the repository as dubious ownership.
var guestExecEnv = []string{
	"--env", "GIT_CONFIG_COUNT=1",
	"--env", "GIT_CONFIG_KEY_0=safe.directory",
	"--env", "GIT_CONFIG_VALUE_0=*",
}
