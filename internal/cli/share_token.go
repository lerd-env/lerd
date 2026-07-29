package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewShareTokenCmd returns the share:token command.
func NewShareTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share:token [token|none]",
		Short: "Show or set the ngrok auth token public shares use",
		Long: `Without an argument, reports whether an ngrok auth token is stored.

With a token, lerd remembers it and uses it for every ngrok share. This is what
lets a machine without ngrok installed share anyway: lerd runs the published
ngrok image instead, and a container carries none of the host's ngrok
configuration, so the token has to come from here.

An installed ngrok uses the token too, so a binary that was never authenticated
with "ngrok config add-authtoken" still works.

"none" forgets the token.

"lerd share --token" still wins for a single run.

The token is a credential: it is stored in the lerd config file, which is
tightened to owner-only the moment one is saved. It is never printed back.`,
		Example: `  lerd share:token
  lerd share:token 2abcXYZ...
  lerd share:token none`,
		Args: cobra.MaximumNArgs(1),
		RunE: runShareToken,
	}
}

func runShareToken(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		if cfg.Share.NgrokToken != "" {
			fmt.Println("set (an ngrok share runs without ngrok installed)")
		} else {
			fmt.Println("none (ngrok has to be installed to share through it)")
		}
		fmt.Println("\nChange it with: lerd share:token <token>|none")
		fmt.Println("Get a token at: https://dashboard.ngrok.com/get-started/your-authtoken")
		return nil
	}

	if strings.EqualFold(args[0], "none") {
		if err := SetShareNgrokToken(""); err != nil {
			return err
		}
		feedback.Begin()
		feedback.Done("ngrok auth token cleared")
		return nil
	}

	token := strings.TrimSpace(args[0])
	if token == "" {
		return fmt.Errorf("the token is empty: pass a token, or \"none\" to forget the stored one")
	}
	if err := SetShareNgrokToken(token); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("ngrok auth token " + feedback.Val("saved"))
	return nil
}
