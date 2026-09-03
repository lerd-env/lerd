package cli

import (
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envpass"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpShellCmd returns the shell command — opens an interactive sh session in the PHP-FPM container.
func NewPhpShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "shell [version]",
		Short:        "Open a shell in the project's PHP-FPM container",
		Long:         "Opens an interactive shell in the PHP-FPM container serving the current project.\nPass a version to open one in that version's shared container instead, from anywhere.",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE:         runPhpShell,
	}
}

// shellWorkDir resolves the directory the shell opens in: the worktree checkout
// when cwd is inside one, otherwise the registered site root, otherwise cwd. A
// worktree nested under its parent path-prefix-matches the parent in
// SiteRootFor, so without the worktree check the shell would open the parent
// tree while running the worktree's own PHP version and FPM image, mixing two
// sites in one session. It resolves the site the same way version detection does.
func shellWorkDir(cwd string) string {
	if wt, _, ok := phpDet.WorktreeRootFor(cwd); ok {
		return wt
	}
	return phpDet.SiteRootFor(cwd)
}

// phpShellInnerScript is what runs inside the container: the opt-in
// in-container bun (lerd php:bun install) on PATH so a bare `bun` resolves,
// harmless when bun isn't installed, then lerd's interactive shell.
func phpShellInnerScript() string {
	return `export PATH="/root/.bun/bin:$PATH"; ` + podman.InteractiveShellScript()
}

// phpShellExecArgs builds the interactive shell exec. It forwards the
// terminal's colour capability so starship and the tools run in the session
// render as they do on the host. An empty workDir leaves the container on its
// own default, which is what a version shell with no project behind it wants.
func phpShellExecArgs(container, workDir string) []string {
	args := append([]string{"exec", "-it"}, terminalColorEnvArgs()...)
	if workDir != "" {
		args = append(args, "-w", workDir)
		args = append(args, envpass.Args(workDir, os.Environ())...)
	}
	return append(args, container, "sh", "-c", phpShellInnerScript())
}

// runVersionShell opens a shell in a version's shared FPM container with no
// project in play, so an image can be looked inside from anywhere. It starts a
// stopped container but never offers to build a missing one: there is no
// project here whose pin would justify a five-minute build.
func runVersionShell(input string) error {
	version, err := config.NormalizePHPVersion(input)
	if err != nil {
		return err
	}
	container := podman.SharedFPMContainerName(version)
	handled, err := startInstalledFPM(version, container)
	if err != nil {
		return err
	}
	if !handled {
		return notInstalledErr(version)
	}
	return runPhpShellExec(container, "")
}

// runPhpShellExec hands the terminal over to the container shell, exiting with
// its status so a shell that ends on an error is not reported as a clean run.
func runPhpShellExec(container, workDir string) error {
	cmd := podman.Cmd(phpShellExecArgs(container, workDir)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

func runPhpShell(_ *cobra.Command, args []string) error {
	if len(args) == 1 {
		return runVersionShell(args[0])
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	version, err := phpVersionForDir(cwd)
	if err != nil {
		return err
	}

	container := fpmContainerForDir(cwd, version)

	version, container, err = ensureFPMRunning(cwd, version, container)
	if err != nil {
		return err
	}

	workDir := shellWorkDir(cwd)

	podman.EnsurePathMounted(workDir, version)
	ensureServicesForCwd(workDir)

	return runPhpShellExec(container, workDir)
}
