package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/p9share"
)

// NewP9ServeCmd returns the hidden `lerd p9-serve` command, which takes the
// arguments of `podman machine server9p` and serves the same folders with a 9p
// server that can replace, append to, lock and release files on Windows.
func NewP9ServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "p9-serve --serve DIR:GUID... PID",
		Short:              "Serve a Hyper-V machine's folders over 9p (internal)",
		Hidden:             true,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(_ *cobra.Command, args []string) error {
			shares, pid, err := p9share.ParseServerArgs(args)
			if err != nil {
				return err
			}
			for _, s := range shares {
				fmt.Fprintf(os.Stderr, "lerd p9-serve: serving %s on hvsock %s\n", s.Dir, s.Service)
			}
			return p9share.Serve(shares, pid)
		},
	}
}
