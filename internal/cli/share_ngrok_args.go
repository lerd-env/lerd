package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewShareNgrokArgsCmd returns the share:ngrok-args command.
func NewShareNgrokArgsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share:ngrok-args [ngrok flags|none]",
		Short: "Show or set the extra flags every ngrok share passes to ngrok",
		Long: `Without an argument, prints the flags stored for ngrok shares.

With flags, every ngrok share passes them to ngrok, from the CLI and from the
dashboard alike. This is for the ngrok features lerd has no setting of its own
for: rewriting the host header, a traffic policy file, basic auth, compression.

A reserved domain is the exception: use "lerd share --domain" for that, which
works the same way for ngrok as it does for Cloudflare Tunnel.

"none" forgets the flags. "lerd share --ngrok-args" still wins for a single run.

A file a flag points at must exist, and on a machine without ngrok installed
(where lerd runs the published image) it is mounted into the container for you.`,
		Example: `  lerd share:ngrok-args
  lerd share:ngrok-args --host-header=rewrite
  lerd share:ngrok-args --traffic-policy-file=/home/me/policy.yml
  lerd share:ngrok-args none`,
		// The whole point of the argument is that it is ngrok's flags, which
		// cobra would otherwise try to parse as its own and reject.
		DisableFlagParsing: true,
		RunE:               runShareNgrokArgs,
	}
}

func runShareNgrokArgs(cmd *cobra.Command, args []string) error {
	// Flag parsing is off, so the one flag this command has of its own is
	// answered here rather than by cobra.
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return cmd.Help()
		}
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		if cfg.Share.NgrokArgs == "" {
			fmt.Println("none (ngrok is run with lerd's own flags only)")
		} else {
			fmt.Println(cfg.Share.NgrokArgs)
		}
		fmt.Println("\nChange it with: lerd share:ngrok-args \"--host-header=rewrite\"|none")
		return nil
	}

	if len(args) == 1 && strings.EqualFold(args[0], "none") {
		if err := SetShareNgrokArgs(""); err != nil {
			return err
		}
		feedback.Begin()
		feedback.Done("ngrok flags cleared")
		return nil
	}

	raw := joinNgrokArgs(args)
	if err := SetShareNgrokArgs(raw); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("ngrok flags set to " + feedback.Val(raw))
	return nil
}

// joinNgrokArgs turns the words the shell handed us back into one stored line.
// An argument that carries a space was quoted on the command line, and has to
// stay quoted or it would come back out of the config as two flags.
func joinNgrokArgs(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			a = `"` + strings.ReplaceAll(a, `"`, "") + `"`
		}
		out = append(out, a)
	}
	return strings.Join(out, " ")
}
