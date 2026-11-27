package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewPhpTrustHostCACmd returns the php:trust-host-ca command, the opt-in that
// copies the root CAs the host added locally into every PHP image.
func NewPhpTrustHostCACmd() *cobra.Command {
	return &cobra.Command{
		Use:   "php:trust-host-ca <on|off>",
		Short: "Trust the root CAs this machine added (a company CA) inside the PHP images",
		Long: "With this on, every PHP image lerd builds also trusts the roots in the host's local\n" +
			"CA folders, such as /usr/local/share/ca-certificates on Debian and Ubuntu, so PHP\n" +
			"reaches internal hosts signed by a company CA. Run 'lerd php:rebuild' to apply it.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(_ *cobra.Command, args []string) error {
			if err := nativeImageCommandRefusal("php:trust-host-ca"); err != nil {
				return err
			}
			return runPhpTrustHostCA(args[0])
		},
	}
}

func runPhpTrustHostCA(mode string) error {
	if mode != "on" && mode != "off" {
		return fmt.Errorf("expected on or off, got %q", mode)
	}
	on := mode == "on"
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) { c.PHP.TrustHostCA = on }); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	feedback.Begin()
	if on {
		feedback.Done("PHP images will trust the root CAs this machine added, run 'lerd php:rebuild' to apply it now")
		return nil
	}
	feedback.Done("PHP images will stop trusting the root CAs this machine added, run 'lerd php:rebuild' to apply it now")
	return nil
}
