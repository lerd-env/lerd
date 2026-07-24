package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// shareTools lists the settable default tunnel tools in the order they are
// offered, with the binary each one needs in PATH. SSH-based tools need no
// dedicated binary beyond ssh itself.
var shareTools = []struct{ name, binary string }{
	{"ngrok", "ngrok"},
	{"cloudflare", "cloudflared"},
	{"expose", "expose"},
	{"serveo", "ssh"},
	{"localhost-run", "ssh"},
}

func shareToolNames() []string {
	names := make([]string, 0, len(shareTools))
	for _, t := range shareTools {
		names = append(names, t.name)
	}
	return names
}

func shareToolBinary(name string) (string, bool) {
	for _, t := range shareTools {
		if t.name == name {
			return t.binary, true
		}
	}
	return "", false
}

// NewShareToolCmd returns the share:tool command.
func NewShareToolCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share:tool [ngrok|cloudflare|expose|serveo|localhost-run|auto]",
		Short: "Show or set the default tunnel tool for lerd share",
		Long: `Without an argument, prints the current default tunnel tool.

With an argument, sets the tool "lerd share" uses when no tool flag is passed.
"auto" clears the default and restores auto-detection
(ngrok, then cloudflared, then Expose, then localhost.run).

The tool must already be installed. A tool flag on "lerd share" still wins for
that run, and "lerd share --domain" always uses Cloudflare Tunnel.`,
		Example: `  lerd share:tool
  lerd share:tool cloudflare
  lerd share:tool auto`,
		Args: cobra.MaximumNArgs(1),
		RunE: runShareTool,
	}
}

func runShareTool(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		if cfg.Share.DefaultTool == "" {
			fmt.Println("auto (detects ngrok, then cloudflared, then Expose, then localhost.run)")
		} else {
			fmt.Println(cfg.Share.DefaultTool)
		}
		fmt.Printf("\nChange it with: lerd share:tool %s|auto\n", strings.Join(shareToolNames(), "|"))
		return nil
	}

	tool := strings.ToLower(args[0])
	if tool == "auto" {
		cfg.Share.DefaultTool = ""
		if err := config.SaveGlobal(cfg); err != nil {
			return err
		}
		feedback.Begin()
		feedback.Done("share tool reset to " + feedback.Val("auto-detect"))
		return nil
	}

	bin, ok := shareToolBinary(tool)
	if !ok {
		return fmt.Errorf("unknown tool %q: use %s, or auto", tool, strings.Join(shareToolNames(), ", "))
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s requires %q which is not in PATH, install it first", tool, bin)
	}

	cfg.Share.DefaultTool = tool
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("default share tool set to " + feedback.Val(tool))
	return nil
}
