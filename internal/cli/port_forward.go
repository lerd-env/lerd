package cli

import (
	"errors"
	"net"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/portforward"
	"github.com/spf13/cobra"
)

// NewPortForwardCmd returns the hidden `lerd port-forward` command a launchd
// job starts on macOS with port 80 or 443 as stdin, relaying to nginx's
// loopback port. launchd binds the port as root and runs this as the user.
func NewPortForwardCmd() *cobra.Command {
	var to int
	cmd := &cobra.Command{
		Use:    "port-forward",
		Short:  "Relay a launchd-held port to nginx (internal)",
		Hidden: true,
		// stdout and stderr are the listening socket too, so nothing is printed.
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if os.Geteuid() == 0 {
				return errors.New("refusing to run as root")
			}
			ln, err := net.FileListener(os.Stdin)
			if err != nil {
				return errors.New("stdin is not a listening socket; launchd starts this command")
			}
			return portforward.Serve(ln, to, lanExposed)
		},
	}
	cmd.Flags().IntVar(&to, "to", 0, "loopback port to relay to")
	cmd.MarkFlagRequired("to") //nolint:errcheck
	return cmd
}

// lanExposed fails closed: an unreadable config keeps the LAN out.
func lanExposed() bool {
	cfg, err := config.LoadGlobal()
	return err == nil && cfg != nil && cfg.LAN.Exposed
}
