package cli

import (
	"os"
	"os/exec"

	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpShellCmd returns the shell command — opens an interactive sh session in the PHP-FPM container.
func NewPhpShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "shell",
		Short:        "Open a shell in the project's PHP-FPM container",
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

func runPhpShell(_ *cobra.Command, _ []string) error {
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

	// Put the opt-in in-container bun (lerd php:bun install) on PATH so a bare
	// `bun` resolves in the shell. Harmless no-op when bun isn't installed.
	cmd := podman.Cmd("exec", "-it", "-w", workDir, container,
		"sh", "-c", `export PATH="/root/.bun/bin:$PATH"; `+podman.InteractiveShellScript())
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
