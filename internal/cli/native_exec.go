package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
)

// nativeExecCommand builds the command that runs PHP for a native site. The
// project is already on the host filesystem at the path the caller gave, so
// there is nothing to mount, stage or rewrite: the binary runs in place.
// PHP_INI_SCAN_DIR carries the same conf.d files the containers mount, so
// php:ini, dumps and xdebug behave identically on either runtime.
func nativeExecCommand(binary, cwd string, args []string, phpVersion string, extraEnv ...string) *exec.Cmd {
	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	env := append(os.Environ(), "PHP_INI_SCAN_DIR="+nativephp.IniScanDir(phpVersion))
	cmd.Env = append(env, extraEnv...)
	return cmd
}

// nativeRuntimeVersion returns the PHP version to run for a directory and
// whether the native runtime is active. It deliberately does not require the
// directory to be a registered site: the php, composer and console shims run
// anywhere, and under this runtime there is no FPM container to fall back to,
// so reaching for one would start the very container the mode exists to avoid.
func nativeRuntimeVersion(cwd string) (string, bool) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.PHPRuntimeMode() != config.PHPRuntimeNative {
		return "", false
	}
	if v, err := phpVersionForDir(cwd); err == nil && v != "" {
		return v, true
	}
	// Outside a project there is no version to detect, so the shim runs the
	// install's default, matching what the container shim would have used.
	if cfg.PHP.DefaultVersion != "" {
		return cfg.PHP.DefaultVersion, true
	}
	return "", false
}

// runNativePHP runs a native site's PHP directly, wiring stdio to the terminal
// so tinker and other interactive commands behave as they do in a container.
// The exit code is returned rather than propagated, matching the podman path.
func runNativePHP(cwd, phpVersion string, args []string, extraEnv []string) (int, error) {
	binary := nativephp.BinaryPath(phpVersion)
	if err := nativephp.EnsureInstalled(phpVersion, binary); err != nil {
		return 0, err
	}
	if err := nativephp.WriteOverrides(phpVersion); err != nil {
		return 0, fmt.Errorf("writing native php overrides: %w", err)
	}
	cmd := nativeExecCommand(binary, cwd, args, phpVersion, extraEnv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// nativeShellRefusal returns the error `lerd shell` should fail with under the
// native runtime, or nil when a container shell is available. The shell exists
// to put you inside the FPM container; under this runtime there is not one, and
// ensuring it would start a container the mode just stopped.
func nativeShellRefusal(cwd string) error {
	if _, ok := nativeRuntimeVersion(cwd); !ok {
		return nil
	}
	return errors.New("there is no container shell under the native runtime: PHP runs on this machine, so use your own shell. Switch back with 'lerd php:runtime container' if you need the container")
}

// nativeImageCommandRefusal returns the error a command that only makes sense
// against the PHP image should fail with under the native runtime, or nil in
// container mode. The native binary's extensions are compiled in and there is
// no Alpine image to install packages into, so these cannot be made to work;
// saying so beats appearing to succeed and changing nothing.
func nativeImageCommandRefusal(command string) error {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.PHPRuntimeMode() != config.PHPRuntimeNative {
		return nil
	}
	return fmt.Errorf("%s needs the container runtime: PHP runs from a binary here, with its extensions compiled in and no image to add packages to. Switch with 'lerd php:runtime container', or keep the site on a version whose native build already carries what you need", command)
}

// nativeTinkerCommand builds the tinker invocation for the native runtime: the
// host binary, run in the project, with the same php arguments and environment
// the container path would have passed through podman exec.
func nativeTinkerCommand(binary, sitePath, phpVersion string, phpArgs, env []string) *exec.Cmd {
	return nativeExecCommand(binary, sitePath, phpArgs, phpVersion, env...)
}
