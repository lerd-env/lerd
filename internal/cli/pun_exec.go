package cli

import (
	"context"
	"os"
	"os/exec"

	"github.com/geodro/lerd/internal/pun"
	"github.com/spf13/cobra"
)

// NewPunCmd returns the pun command: the pun PHP toolchain (format, lint,
// analyse, composer installs) at lerd's pinned version, fetched on first use.
func NewPunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "pun [args...]",
		Short:              "Run pun, the PHP formatter, linter, analyser and installer",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runPun(args)
		},
	}
}

func runPun(args []string) error {
	bin, err := pun.Ensure(context.Background(), os.Stderr)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	c := exec.Command(bin, args...)
	c.Dir = cwd
	c.Env = append(os.Environ(), "PATH="+hostVendorBinPath(cwd))
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return runHostAndReap(c)
}
