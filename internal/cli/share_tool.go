package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// shareToolBinaries maps a default-tool name to the binary it needs in PATH.
// SSH-based tools need no dedicated binary beyond ssh itself.
var shareToolBinaries = map[string]string{
	"ngrok":         "ngrok",
	"cloudflare":    "cloudflared",
	"expose":        "expose",
	"serveo":        "ssh",
	"localhost-run": "ssh",
}

// NewShareToolCmd returns the share:tool command.
func NewShareToolCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share:tool [ngrok|cloudflare|expose|serveo|localhost-run|auto]",
		Short: "Show or set the default tunnel tool for lerd share",
		Long: `Without an argument, prints the current default tunnel tool.

With an argument, sets the tool "lerd share" uses when no tool flag is passed.
"auto" clears the default and restores auto-detection
(ngrok, then cloudflared, then Expose, then localhost.run).`,
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

	bin, ok := shareToolBinaries[tool]
	if !ok {
		return fmt.Errorf("unknown tool %q: use ngrok, cloudflare, expose, serveo, localhost-run, or auto", tool)
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
