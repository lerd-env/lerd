package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// shareTokenProviders maps the provider argument of share:token to what the
// token does and where it is stored. ngrok is the implied provider when the
// argument is a bare token, which is what the command accepted first.
var shareTokenProviders = map[string]struct {
	set      func(string) error
	stored   func(*config.GlobalConfig) string
	setNote  string
	noneNote string
	tokenURL string
}{
	"ngrok": {
		set:      SetShareNgrokToken,
		stored:   func(cfg *config.GlobalConfig) string { return cfg.Share.NgrokToken },
		setNote:  "set (an ngrok share runs without ngrok installed)",
		noneNote: "none (ngrok has to be installed to share through it)",
		tokenURL: "https://dashboard.ngrok.com/get-started/your-authtoken",
	},
	"pinggy": {
		set:      SetSharePinggyToken,
		stored:   func(cfg *config.GlobalConfig) string { return cfg.Share.PinggyToken },
		setNote:  "set (a Pinggy share keeps its stable subdomain)",
		noneNote: "none (a Pinggy share gets an ephemeral free-tier URL)",
		tokenURL: "https://dashboard.pinggy.io",
	},
}

// NewShareTokenCmd returns the share:token command.
func NewShareTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share:token [ngrok|pinggy] [token|none]",
		Short: "Show or set the auth tokens public shares use",
		Long: `Without arguments, reports whether an auth token is stored for each provider.

A bare token means ngrok, which is what lets a machine without ngrok installed
share anyway: lerd runs the published ngrok image instead, and a container
carries none of the host's ngrok configuration, so the token has to come from
here. An installed ngrok uses the token too, so a binary that was never
authenticated with "ngrok config add-authtoken" still works.

"lerd share:token pinggy <token>" stores a Pinggy access token instead, which
gives a Pinggy share a stable subdomain rather than an ephemeral free-tier URL.

"none" forgets the provider's token.

"lerd share --token" still wins for a single run.

A token is a credential: it is stored in the lerd config file, which is
tightened to owner-only the moment one is saved. It is never printed back.`,
		Example: `  lerd share:token
  lerd share:token 2abcXYZ...
  lerd share:token pinggy 2abcXYZ...
  lerd share:token pinggy none`,
		Args: cobra.MaximumNArgs(2),
		RunE: runShareToken,
	}
}

func runShareToken(_ *cobra.Command, args []string) error {
	// The provider argument is optional and a bare token keeps meaning ngrok,
	// so the first argument is a provider only when it names one.
	provider := "ngrok"
	if len(args) > 0 {
		if _, ok := shareTokenProviders[strings.ToLower(args[0])]; ok {
			provider = strings.ToLower(args[0])
			args = args[1:]
		} else if len(args) == 2 {
			return fmt.Errorf("unknown provider %q: use ngrok or pinggy", args[0])
		}
	}
	p := shareTokenProviders[provider]

	if len(args) == 0 {
		cfg, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		for _, name := range []string{"ngrok", "pinggy"} {
			sp := shareTokenProviders[name]
			status := sp.noneNote
			if sp.stored(cfg) != "" {
				status = sp.setNote
			}
			fmt.Printf("%s: %s\n", name, status)
		}
		fmt.Println("\nChange it with: lerd share:token [ngrok|pinggy] <token>|none")
		fmt.Println("Get a token at: https://dashboard.ngrok.com/get-started/your-authtoken (ngrok)")
		fmt.Println("                https://dashboard.pinggy.io (Pinggy)")
		return nil
	}

	if strings.EqualFold(args[0], "none") {
		if err := p.set(""); err != nil {
			return err
		}
		feedback.Begin()
		feedback.Done(provider + " auth token cleared")
		return nil
	}

	token := strings.TrimSpace(args[0])
	if token == "" {
		return fmt.Errorf("the token is empty: pass a token, or \"none\" to forget the stored one")
	}
	if err := p.set(token); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done(provider + " auth token " + feedback.Val("saved"))
	return nil
}
