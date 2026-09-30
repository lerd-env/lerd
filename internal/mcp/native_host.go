package mcp

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/agentenv"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
)

// runHostCmd runs a command built by runOnHost; a var so tests can see what
// would run without a real php.
var runHostCmd = func(c *exec.Cmd) error { return c.Run() }

// nativeRuntime reports whether PHP runs on the host, where there is no FPM
// container to exec into and starting one would undo the runtime.
func nativeRuntime() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative
}

// runOnHost runs argv in dir the way the podman exec would have, with output
// captured into out.
func runOnHost(dir string, argv []string, out *bytes.Buffer, extraEnv ...string) error {
	env := append(agentenv.MCPInject(os.Environ()), extraEnv...)
	c, err := nativephp.HostCommand(dir, argv, env...)
	if err != nil {
		return err
	}
	c.Stdout, c.Stderr = out, out
	return runHostCmd(c)
}
