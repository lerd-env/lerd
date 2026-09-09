package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// runGlobalHostCLI runs a globally installed composer binary on lerd's own PHP
// rather than in the container, when the store says it cannot work there, and
// reports whether it took the call. The case it exists for is a CLI that
// authenticates over a browser callback: it binds a loopback listener on a port
// it picks per run, and the browser dialling 127.0.0.1 on the host reaches
// nothing, because the listener is in the container's network namespace.
//
// This is the global sibling of a framework's host_commands. A tool composer
// installed globally belongs to no project, so nothing about it can be resolved
// through the framework of whatever directory it happens to be run from, and
// the declaration is matched against the binary's name instead.
func runGlobalHostCLI(cwd string, args []string, extraEnv []string) (int, bool, error) {
	i := phpScriptArgIndex(args)
	if i < 0 || i >= len(args) {
		return 0, false, nil
	}
	script := args[i]
	if filepath.Dir(script) != composerGlobalBinDir() {
		return 0, false, nil
	}
	// Under the native runtime PHP already runs on the host, so there is no
	// boundary to escape and the native path is the one that knows which
	// extensions and ini this machine's PHP was set up with.
	if _, native := nativeRuntimeVersion(cwd); native {
		return 0, false, nil
	}
	if !config.GlobalHostBinary(composerHomeDir(), filepath.Base(script)) {
		return 0, false, nil
	}
	// lerd's own pinned build rather than whatever php the machine happens to
	// carry: a lerd install is not required to have a host PHP at all, and the
	// version the project resolves to is the one the tool's code expects.
	version, err := phpVersionForDir(cwd)
	if err != nil {
		return 0, true, err
	}
	php, err := ensureHostPHPBinary(os.Stderr, version)
	if err != nil {
		return 0, true, fmt.Errorf("%s cannot run inside the PHP container, and lerd has no PHP on this machine to run it with: %w", filepath.Base(script), err)
	}
	cmd := exec.Command(php, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), true, nil
		}
		return 0, true, err
	}
	return 0, true, nil
}
