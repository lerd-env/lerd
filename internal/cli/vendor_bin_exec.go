package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/agentenv"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envpass"
	"github.com/geodro/lerd/internal/podman"
	"golang.org/x/term"
)

// vendorBinIsPHP reports whether a composer binary should be handed to `php`.
// wp-cli and drush ship a POSIX shell wrapper as their vendor/bin entry, and
// running one through php prints its source instead of executing it. Anything
// without a shebang, or that names php in one, keeps the php route, so an
// unreadable or absent file fails the way it always did.
func vendorBinIsPHP(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 128)
	n, _ := f.Read(buf)
	head := string(buf[:n])
	if !strings.HasPrefix(head, "#!") {
		return true
	}
	line, _, _ := strings.Cut(head, "\n")
	return strings.Contains(line, "php")
}

// containerExecEnvArgs builds the `--env` flags every exec into a project's FPM
// container needs: the composer identity, a PATH that reaches both the
// project's and the global composer binaries, the site tag for the debug
// bridge, terminal colour, and the host variables lerd forwards. Shared with
// RunPHPVersionCaptureEnv so the two exec routes cannot drift.
func containerExecEnvArgs(cwd string) []string {
	home := os.Getenv("HOME")
	composerHome := os.Getenv("COMPOSER_HOME")
	if composerHome == "" {
		// Respect XDG: prefer ~/.config/composer, fall back to ~/.composer
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig == "" {
			xdgConfig = filepath.Join(home, ".config")
		}
		composerHome = filepath.Join(xdgConfig, "composer")
	}
	composerBin := filepath.Join(composerHome, "vendor", "bin")
	projectVendorBin := filepath.Join(cwd, "vendor", "bin")

	args := []string{
		"--env", "HOME=" + home,
		"--env", "COMPOSER_HOME=" + composerHome,
		"--env", "PATH=" + projectVendorBin + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:" + composerBin,
	}
	args = append(args, debugSiteEnvArgs(cwd)...)
	args = append(args, terminalColorEnvArgs()...)
	// Forward SPX_* profiler vars from the host so `SPX_ENABLED=1 php ...` (or
	// any shim'd tool like composer) reaches SPX inside the container.
	for _, e := range spxPassthroughEnv(os.Environ()) {
		args = append(args, "--env", e)
	}
	// Forward AI agent detection vars so agent-detector (e.g. laravel/pao)
	// still emits JSON when run inside the container.
	for _, e := range agentenv.Passthrough(os.Environ()) {
		args = append(args, "--env", e)
	}
	// Forward the host variables an external environment provider (a secrets
	// manager, direnv) injected into lerd's own process. Names only: podman
	// reads each value out of lerd's environment, so no secret is in argv.
	args = append(args, envpass.Args(cwd, os.Environ())...)
	// Point composer/git at the shared ssh-agent when it's running, so private
	// packages with passphrase-protected keys authenticate over SSH. No-op when
	// the agent isn't up (falls back to the bind-mounted on-disk keys).
	return append(args, podman.SSHAuthSockEnv()...)
}

// vendorBinExecArgs builds the podman exec that runs a non-PHP composer binary
// directly in the project's container. rel is the binary's path relative to
// cwd, which is also the exec's working directory.
func vendorBinExecArgs(cwd, container, rel string, args []string, tty bool) []string {
	execFlags := []string{"exec", "-i"}
	if tty {
		execFlags = append(execFlags, "-t")
	}
	cmdArgs := append(execFlags, "-w", cwd)
	cmdArgs = append(cmdArgs, containerExecEnvArgs(cwd)...)
	cmdArgs = append(cmdArgs, container, rel)
	return append(cmdArgs, args...)
}

// RunVendorBin runs a composer binary at rel (relative to cwd), choosing
// between php and a direct exec by what the file's shebang says it is.
func RunVendorBin(cwd, rel string, args []string) error {
	if !vendorBinIsPHP(filepath.Join(cwd, rel)) {
		return runVendorBinDirect(cwd, rel, args)
	}
	return RunPHP(cwd, append([]string{rel}, args...))
}

// runVendorBinDirect execs a composer binary in the project's container without
// putting php in front of it, for the wrappers php cannot run.
func runVendorBinDirect(cwd, rel string, args []string) error {
	version, err := phpVersionForDir(cwd)
	if err != nil {
		return err
	}
	recordCwdActivity(cwd)

	// Under the native runtime there is no container, and the wrapper resolves
	// php off the host PATH lerd's shim dir already provides.
	if _, native := nativeRuntimeVersion(cwd); native {
		c := exec.Command(filepath.Join(cwd, rel), args...)
		c.Dir = cwd
		c.Env = append(os.Environ(), "PATH="+config.PathWithBinDir())
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return runAndPropagate(c)
	}

	container := fpmContainerForDir(cwd, version)
	version, container, err = ensureFPMRunning(cwd, version, container)
	if err != nil {
		return err
	}
	podman.EnsurePathMounted(cwd, version)
	ensureServicesForCwd(cwd)

	cmd := podman.Cmd(vendorBinExecArgs(cwd, container, rel, args, term.IsTerminal(int(os.Stdin.Fd())))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return runAndPropagate(cmd)
}

// runAndPropagate runs cmd and exits with the child's status, so a failing
// composer binary fails the shell that called lerd.
func runAndPropagate(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}
