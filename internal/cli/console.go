package cli

import (
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envpass"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewConsoleCmd returns the console command — runs framework console in the project's container.
func NewConsoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "console [args...]",
		Aliases:            []string{"artisan", "a"},
		Short:              "Run framework console command in the project's container",
		Example:            "  lerd console cache:clear\n  lerd console make:controller User",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE:               runConsole,
	}
}

// consoleCmdArgs builds the podman exec that runs a framework console command
// in the project's container, carrying the terminal's colour capability so
// styled console output keeps the fidelity it has on the host.
func consoleCmdArgs(cwd, container, consoleCmd string, tty bool, args []string) []string {
	execFlags := []string{"exec", "-i"}
	if tty {
		execFlags = append(execFlags, "-t")
	}
	cmdArgs := append(execFlags, terminalColorEnvArgs()...)
	cmdArgs = append(cmdArgs, envpass.Args(cwd, os.Environ())...)
	cmdArgs = append(cmdArgs, "-w", cwd, container, "php", consoleCmd)
	return append(cmdArgs, args...)
}

func runConsole(_ *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Get console command for current framework
	consoleCmd, err := config.GetConsoleCommand(cwd)
	if err != nil {
		return err
	}

	// `lerd artisan native:run` has to reach the same host binary `lerd php
	// artisan native:run` does, so the console name is put back in front of the
	// arguments and the declaration is matched on the command as typed.
	if code, took, err := runDeclaredHostCommand(cwd, append([]string{consoleCmd}, args...), nil); took {
		if err != nil {
			return err
		}
		if code != 0 {
			os.Exit(code)
		}
		return nil
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

	podman.EnsurePathMounted(cwd, version)
	ensureServicesForCwd(cwd)

	cmd := podman.Cmd(consoleCmdArgs(cwd, container, consoleCmd, term.IsTerminal(int(os.Stdin.Fd())), args)...)
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
